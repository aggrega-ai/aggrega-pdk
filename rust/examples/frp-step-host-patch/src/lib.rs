// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Reference cross-entity (`/host` PATCH) feature-recipe-pipeline step
//! plugin for the Aggrega Wasm runtime.
//!
//! Where the `frp-step-patch` fixture patches its OWN output and only
//! EMITs into the host set, this plugin exercises the cross-entity PATCH
//! contract: a component-scope FeatureRecipe modifies the documents of the
//! component it is attached to (`/host`). It is the credential/guardrail
//! injector shape — a Feature that has to reach into its consumer's
//! existing workload, which an additive EMIT cannot do.
//!
//!  - It content-selects the host component's chart `values.yaml` from the
//!    keyed host document set (the structured unit that carries chart
//!    values — an `image` key and no `apiVersion`), learns that unit's key
//!    at runtime, and roots a `/host/<key>` PATCH at it — injecting an
//!    environment variable under `environment`.
//!  - It declares — in `describe()` — `operates=patch`, `scope=host`, and
//!    `helm-chart` as its sole compatible provider: it patches chart
//!    source, so it is tested-compatible only with the provider that emits
//!    chart source. The host checks this against the HOST component's
//!    provider (the provider rendering the `/host` documents) before any
//!    guest runs.
//!
//! The PATCH target is `values.yaml` deliberately: a chart's `values.yaml`
//! carries no Go-template actions, so the helm-chart codec delegates it to
//! the stock node-span YAML codec and it is cleanly RFC-6902-patchable. A
//! chart template under `templates/` that mixes control-flow actions is an
//! opaque unit, not a PATCH target, and the host halts loudly on a PATCH
//! against it — the operator patches `values.yaml` instead.

use aggrega_pdk_rust::{
    encode_descriptor, PluginDescriptor, PluginDescriptorMeta, PluginExport, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::BTreeMap;

/// The static `describe()` self-report: a v1alpha1 `frp_step` plugin whose
/// `step` export patches the host component's chart source and is
/// tested-compatible only with the `helm-chart` provider (the provider that
/// emits chart source).
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "frp_step".to_string(),
        capabilities: vec!["helm-chart".to_string()],
        exports: vec![PluginExport {
            name: "step".to_string(),
            operates: "patch".to_string(),
            scope: "host".to_string(),
            selector: "chart values.yaml of the attached host component".to_string(),
            ..Default::default()
        }],
        metadata: Some(Box::new(PluginDescriptorMeta {
            author: "aggrega".to_string(),
            license: "Apache-2.0".to_string(),
            ..Default::default()
        })),
        ..Default::default()
    }
}

/// One addressable unit the host hands the guest. A structured unit carries
/// a `tree` (the RFC-6902-addressable view the guest content-selects and
/// patches); an opaque unit carries no `tree`, so it is never a PATCH
/// target. The token rides the wire but the guest reads only the tree to
/// select.
#[derive(Deserialize)]
struct Unit {
    #[serde(default)]
    tree: Option<Value>,
}

/// The keyed, scope-rooted unit set the host hands the guest. A
/// component-scope FeatureRecipe attached to a host receives both roots;
/// this guest acts only on `host`.
#[derive(Deserialize)]
struct DocumentSet {
    #[serde(default)]
    own: BTreeMap<String, Unit>,
    #[serde(default)]
    host: BTreeMap<String, Unit>,
}

/// The subset of the host's FRPStepInput this plugin reads.
#[derive(Deserialize)]
struct StepInput {
    documents: DocumentSet,
}

/// One RFC 6902 patch addressed to a document by scope and key.
#[derive(Serialize)]
struct JsonPatchOp {
    scope: String,
    key: String,
    op: String,
    path: String,
    value: Value,
}

/// The host's FRPStepOutput wire shape: key-addressed patches plus
/// additive side effects (unused by this guest).
#[derive(Serialize, Default)]
struct StepOutput {
    #[serde(skip_serializing_if = "Vec::is_empty")]
    patches: Vec<JsonPatchOp>,
}

/// The host calls `describe()` once at load to read the plugin's
/// self-declaration before dispatching `step`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(encode_descriptor(&descriptor()))
}

/// Reports whether a unit's tree is the host chart's `values.yaml`: an
/// object carrying an `image` key and NOT an `apiVersion` key. `Chart.yaml`
/// carries `apiVersion`; rendered K8s manifests under `templates/` carry
/// `apiVersion`; only `values.yaml` matches. Selection is by content, never
/// by filename — the guest cannot know the emitted key ahead of time.
fn is_chart_values(tree: &Value) -> bool {
    match tree {
        Value::Object(map) => map.contains_key("image") && !map.contains_key("apiVersion"),
        _ => false,
    }
}

/// `step` runs one cross-entity pipeline step over the keyed document set:
/// it content-selects the host chart's `values.yaml` and PATCHes it,
/// injecting an environment variable into the host workload's `environment`
/// block. Iterating a BTreeMap keeps selection deterministic across runs.
#[plugin_fn]
pub fn step(input: String) -> FnResult<String> {
    let parsed: StepInput = serde_json::from_str(&input)?;
    let _ = &parsed.documents.own; // this guest acts only on the host root.

    let mut out = StepOutput::default();

    for (key, unit) in parsed.documents.host.iter() {
        let tree = match unit.tree.as_ref() {
            Some(t) => t,
            None => continue, // opaque unit (a templated file): never a PATCH target.
        };
        if !is_chart_values(tree) {
            continue;
        }
        // Inject an env var into the host chart's `environment` map. The
        // host's `values.yaml` ships `environment: {}`, so an `add` of a new
        // member is a clean node-span insertion the stock codec splices in
        // without disturbing sibling keys, comments, or styles.
        out.patches.push(JsonPatchOp {
            scope: "host".to_string(),
            key: key.clone(),
            op: "add".to_string(),
            path: "/environment/INJECTED_BY_FEATURE".to_string(),
            value: Value::String("frp-step-host-patch".to_string()),
        });
    }

    Ok(serde_json::to_string(&out)?)
}
