// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Reference render-stage policy plugin that runs a Kubewarden-style
//! validating policy over the rendered Kubernetes manifests.
//!
//! Kubewarden policies are written against the Kubernetes AdmissionReview
//! contract: a policy receives a `ValidationRequest` wrapping the admission
//! request object and returns a `ValidationResponse` that accepts or rejects
//! it. At the render stage the compiler's output already *is* a set of
//! Kubernetes resources, so this adapter bridges the two contracts entirely
//! in-guest:
//!
//!   - Outward, it speaks the Aggrega plugin ABI: `describe()` reports a
//!     `policy` plugin evaluating the `render` stage, and `validate`
//!     consumes the render-stage payload and returns policy verdicts. The host
//!     stays free of any Kubewarden or admission-control logic.
//!   - Inward, it absorbs the Kubewarden contract: for each rendered object it
//!     synthesizes a `ValidationRequest`, runs an ordinary in-guest policy
//!     function that returns a `ValidationResponse`, and maps a rejection to a
//!     verdict.
//!
//! Wire types. The `ValidationRequest` / `ValidationResponse` types below are
//! local structs shaped exactly per the published Kubewarden Rust policy SDK
//! (`kubewarden-policy-sdk` v0.17.3, `src/request.rs` and `src/response.rs`):
//! the same field names, the same JSON representation, the same admission
//! request layout. They are reproduced locally rather than pulled from the SDK
//! crate because the SDK depends on `wapc-guest`, whose unconditional
//! `#[no_mangle] __guest_call` export forces a `wapc` host-module import — the
//! waPC guest ABI — that this runtime's plugin loader does not host. Our own
//! `validate` export is the sole entry point, and the local types keep the wire
//! contract identical to a real Kubewarden policy without linking the waPC ABI.
//!
//! Reference policy. The bundled policy is a require-label rule — a common
//! Kubewarden pattern: every object must carry a specific label, named by the
//! attachment's `settings.required_label`. An object missing the label is
//! rejected; an object carrying it is accepted. A rejection becomes one verdict
//! per rejected object; an acceptance emits nothing, so a fully compliant
//! render produces no verdicts and the build proceeds unchanged.
//!
//! Verdict severity. A rejection maps to a `deny` verdict by default, which
//! halts the build. The attachment may set `settings.severity` to `warn` or
//! `audit` to downgrade the mapping — a warn flags the build and lets it
//! proceed, an audit records without affecting the outcome. Any other value
//! (or none) maps to `deny`.
//!
//! Determinism. The host hands the artifacts already sorted by path; the guest
//! walks them in that order, walks each artifact's documents in document order,
//! and emits verdicts in that order. No map iteration enters the emit path, so
//! identical input yields a byte-identical verdict sequence.

use std::collections::{HashMap, HashSet};

use aggrega_abi::RenderPolicyInput;
use aggrega_pdk_rust::{PolicyResult, PolicyOutput};
use extism_pdk::{plugin_fn, FnResult};
use serde::{Deserialize, Serialize};

/// The static `describe()` self-report: a v1alpha1 `policy` plugin
/// evaluating the render stage, whose `validate` export the host validates
/// before dispatch.
const PLUGIN_DESCRIPTOR: &str = r#"{"abi_version":"v1alpha1","plugin_type":"policy","stage":"render","description":"Run a Kubewarden-style validating policy over the rendered Kubernetes manifests.","exports":[{"name":"validate"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}"#;

/// The host calls `describe()` once at load to read the plugin's static
/// self-report before dispatching `validate`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(PLUGIN_DESCRIPTOR.to_string())
}

/// The host calls `validate(input)` with the render-stage payload as a JSON
/// string and reads back the fired verdicts. A malformed payload propagates as
/// an error, which the host surfaces as a build fault.
#[plugin_fn]
pub fn validate(input: String) -> FnResult<String> {
    let payload: RenderPolicyInput = serde_json::from_str(&input)?;
    let settings: AdapterSettings = match &payload.settings {
        Some(raw) => serde_json::from_str(raw.get())?,
        None => AdapterSettings::default(),
    };
    let severity = settings.severity();

    let mut results = Vec::new();
    // Artifacts arrive path-sorted from the host; walk them in that order and
    // each artifact's documents in document order so the verdict sequence is
    // deterministic.
    for artifact in &payload.artifacts {
        for document in serde_yaml::Deserializer::from_str(&artifact.content) {
            let value = serde_json::Value::deserialize(document)?;
            // Skip anything that is not a Kubernetes object: empty documents and
            // mappings without a `kind`. This adapter is attached to k8s-render
            // output; a non-object document carries nothing to validate.
            if !is_kubernetes_object(&value) {
                continue;
            }
            let request = build_request(&value, &settings.required_label);
            let response = evaluate(&request);
            if !response.accepted {
                results.push(PolicyResult {
                    severity: severity.to_string(),
                    message: format!(
                        "{}: {}",
                        object_identity(&value),
                        response.message.unwrap_or_default()
                    ),
                });
            }
        }
    }
    Ok(serde_json::to_string(&PolicyOutput { results })?)
}

/// AdapterSettings is the attachment's free-form settings object as this
/// adapter reads it. `required_label` is the Kubewarden policy's own setting;
/// `severity` is the adapter's mapping knob for how a rejection lands as a
/// verdict. Both default to their zero value when the attachment omits them.
#[derive(Debug, Default, Deserialize)]
struct AdapterSettings {
    #[serde(default)]
    required_label: String,
    #[serde(default)]
    severity: Option<String>,
}

impl AdapterSettings {
    /// severity resolves the verdict severity a rejection maps to: `warn` or
    /// `audit` when the attachment selects one, else `deny`.
    fn severity(&self) -> &str {
        match self.severity.as_deref() {
            Some("warn") => "warn",
            Some("audit") => "audit",
            _ => "deny",
        }
    }
}

/// PolicySettings is the Kubewarden policy's own settings shape — the slice of
/// the attachment settings the policy function reads. Keeping it separate from
/// [`AdapterSettings`] draws the line between what the Kubewarden policy sees
/// (its settings) and what the adapter consumes (the severity mapping).
#[derive(Debug, Default, Deserialize)]
struct PolicySettings {
    required_label: String,
}

/// build_request synthesizes a Kubewarden `ValidationRequest` for one rendered
/// object. The admission fields are derived from the parsed object itself — the
/// kind and API group/version from `apiVersion`/`kind`, the name and namespace
/// from `metadata` — never from the artifact's file path. The operation is
/// `CREATE`, the shape a render-time admission of freshly emitted manifests
/// takes.
fn build_request(object: &serde_json::Value, required_label: &str) -> ValidationRequest<PolicySettings> {
    let kind = string_at(object, &["kind"]);
    let (group, version) = split_api_version(&string_at(object, &["apiVersion"]));
    ValidationRequest {
        settings: PolicySettings {
            required_label: required_label.to_string(),
        },
        request: KubernetesAdmissionRequest {
            kind: GroupVersionKind {
                group: group.clone(),
                version: version.clone(),
                kind: kind.clone(),
            },
            resource: GroupVersionResource {
                group,
                version,
                kind: kind.to_lowercase(),
            },
            name: string_at(object, &["metadata", "name"]),
            namespace: string_at(object, &["metadata", "namespace"]),
            operation: "CREATE".to_string(),
            object: object.clone(),
            ..Default::default()
        },
    }
}

/// evaluate is the reference Kubewarden validating policy, called as an ordinary
/// in-guest function. It requires every object to carry the label named in
/// settings; an object missing it is rejected. An empty `required_label` imposes
/// no requirement, so the policy accepts. The policy validates only — it never
/// sets `mutated_object`.
fn evaluate(request: &ValidationRequest<PolicySettings>) -> ValidationResponse {
    let required = &request.settings.required_label;
    if required.is_empty() {
        return accept();
    }
    let present = request
        .request
        .object
        .get("metadata")
        .and_then(|metadata| metadata.get("labels"))
        .and_then(|labels| labels.get(required))
        .is_some();
    if present {
        accept()
    } else {
        reject(format!("required label {required:?} is missing"))
    }
}

/// accept builds an accepting Kubewarden response.
fn accept() -> ValidationResponse {
    ValidationResponse {
        accepted: true,
        ..Default::default()
    }
}

/// reject builds a rejecting Kubewarden response carrying the reason message.
fn reject(message: String) -> ValidationResponse {
    ValidationResponse {
        accepted: false,
        message: Some(message),
        ..Default::default()
    }
}

/// is_kubernetes_object reports whether a parsed YAML document is a Kubernetes
/// object this adapter validates: a mapping carrying a `kind`.
fn is_kubernetes_object(value: &serde_json::Value) -> bool {
    value
        .as_object()
        .map(|object| object.contains_key("kind"))
        .unwrap_or(false)
}

/// object_identity renders a human-readable identity for an object — its kind,
/// name, and namespace when present — for the verdict message.
fn object_identity(object: &serde_json::Value) -> String {
    let kind = string_at(object, &["kind"]);
    let name = string_at(object, &["metadata", "name"]);
    let namespace = string_at(object, &["metadata", "namespace"]);
    if namespace.is_empty() {
        format!("{kind} {name:?}")
    } else {
        format!("{kind} {name:?} in namespace {namespace:?}")
    }
}

/// string_at reads a nested string field by key path, returning the empty
/// string when any segment is absent or the leaf is not a string.
fn string_at(value: &serde_json::Value, path: &[&str]) -> String {
    let mut current = value;
    for key in path {
        match current.get(key) {
            Some(next) => current = next,
            None => return String::new(),
        }
    }
    current.as_str().unwrap_or("").to_string()
}

/// split_api_version splits a Kubernetes `apiVersion` into its group and
/// version. A core-group version (no slash, e.g. `v1`) yields an empty group.
fn split_api_version(api_version: &str) -> (String, String) {
    match api_version.split_once('/') {
        Some((group, version)) => (group.to_string(), version.to_string()),
        None => (String::new(), api_version.to_string()),
    }
}

// --- Kubewarden wire types --------------------------------------------------
//
// Local reproductions shaped exactly per the published Kubewarden Rust policy
// SDK, kubewarden-policy-sdk v0.17.3 (src/request.rs, src/response.rs). Field
// names and JSON representation match the SDK's types so the in-guest policy
// sees the same ValidationRequest a real Kubewarden host would deliver and
// returns the same ValidationResponse. They are reproduced locally to avoid
// linking the SDK's waPC guest ABI (see the module-level note above).

/// ValidationRequest holds the data provided to the policy at evaluation time:
/// the policy settings and the Kubernetes admission request.
#[derive(Debug, Serialize, Deserialize)]
struct ValidationRequest<T: Default> {
    settings: T,
    request: KubernetesAdmissionRequest,
}

/// KubernetesAdmissionRequest is the Kubernetes AdmissionReview request the
/// policy evaluates.
#[derive(Debug, Default, Serialize, Deserialize)]
#[serde(default)]
struct KubernetesAdmissionRequest {
    uid: String,
    kind: GroupVersionKind,
    resource: GroupVersionResource,
    #[serde(alias = "subResource")]
    sub_resource: String,
    #[serde(alias = "requestKind")]
    request_kind: GroupVersionKind,
    #[serde(alias = "requestResource")]
    request_resource: GroupVersionResource,
    #[serde(alias = "requestSubResource")]
    request_sub_resource: String,
    name: String,
    namespace: String,
    operation: String,
    #[serde(alias = "userInfo")]
    user_info: UserInfo,
    object: serde_json::Value,
    #[serde(alias = "oldObject")]
    old_object: serde_json::Value,
    #[serde(alias = "dryRun")]
    dry_run: bool,
    options: HashMap<String, serde_json::Value>,
}

/// GroupVersionKind unambiguously identifies a kind.
#[derive(Debug, Default, Serialize, Deserialize)]
#[serde(default)]
struct GroupVersionKind {
    group: String,
    version: String,
    kind: String,
}

/// GroupVersionResource unambiguously identifies a resource.
#[derive(Debug, Default, Serialize, Deserialize)]
#[serde(default)]
struct GroupVersionResource {
    group: String,
    version: String,
    kind: String,
}

/// UserInfo holds information about the user who made the request.
#[derive(Debug, Default, Serialize, Deserialize)]
#[serde(default)]
struct UserInfo {
    username: String,
    uid: String,
    groups: HashSet<String>,
    extra: HashMap<String, serde_json::Value>,
}

/// ValidationResponse holds the outcome of policy evaluation. This adapter's
/// policy validates only: it never sets `mutated_object`.
#[derive(Debug, Default, Serialize, Deserialize)]
struct ValidationResponse {
    accepted: bool,
    message: Option<String>,
    code: Option<u16>,
    mutated_object: Option<serde_json::Value>,
    audit_annotations: Option<HashMap<String, String>>,
    warnings: Option<Vec<String>>,
}
