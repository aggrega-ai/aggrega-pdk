// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

//! Reference cross-entity (`/host` PATCH) feature-recipe-pipeline step
//! plugin for the compose provider.
//!
//! The compose provider is an assembler: each component renders a
//! pre-assembly fragment, and the host merges every fragment into one
//! `docker-compose.yaml` at assembly time. A component-scope FeatureRecipe's
//! `/host` PATCH therefore cannot run per-node — the host's documents are not
//! discrete until assembly. The host defers the PATCH and replays this plugin
//! in the post-assembly phase against the merged document, where the service
//! the FeatureRecipe is attached to is a discrete, addressable unit.
//!
//!  - It content-selects the merged compose document from the keyed host
//!    document set: the structured unit whose tree carries a top-level
//!    `services` mapping. Selection is by content, never by filename — the
//!    guest cannot know the emitted key ahead of time.
//!  - For each service in that document it injects an environment variable
//!    under the service's `environment` block, rooting a `/host/<key>` PATCH
//!    at the unit the guest learned at runtime.
//!  - It declares — in `describe()` — `operates=patch`, `scope=host`, and
//!    `compose` as its sole compatible provider: it patches a merged compose
//!    document, so it is tested-compatible only with the provider that emits
//!    one. The host checks this against the HOST component's provider before
//!    any guest runs.
//!
//! The PATCH target is the merged `docker-compose.yaml`: a plain YAML
//! document the compose provider hands the stock node-span codec, so it is
//! cleanly RFC-6902-patchable and the surgical insert preserves every sibling
//! service, key, comment, and scalar style verbatim.
//!
//! The pipeline-step socket carries canonical CBOR in both directions, so
//! `step` takes and returns bytes: it reads its input with
//! `FRPStepInput::decode_cbor` and returns its output's `encode_cbor()`,
//! through the codec generated into the `aggrega-abi` crate.

use aggrega_abi::{FRPStepInput, FRPStepOutput, JSONPatchOp};
use aggrega_pdk_rust::{
    encode_descriptor, PluginDescriptor, PluginDescriptorMeta, PluginExport, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};
use serde_json::Value;

/// The static `describe()` self-report: a `pipeline-step` plugin whose
/// `step` export patches the merged compose document and is tested-compatible
/// only with the `compose` provider.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "pipeline-step".to_string(),
        capabilities: vec!["compose".to_string()],
        exports: vec![PluginExport {
            name: "step".to_string(),
            operates: "patch".to_string(),
            scope: "host".to_string(),
            selector: "environment of the attached host's service in the merged compose document"
                .to_string(),
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

/// The host calls `describe()` once at load to read the plugin's
/// self-declaration before dispatching `step`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(encode_descriptor(&descriptor()))
}

/// Returns the sorted service names of a merged compose document's tree: an
/// object carrying a top-level `services` mapping. Any other tree (a fragment,
/// a non-compose document) yields no names. Sorting keeps the emitted patch
/// order deterministic across runs regardless of the tree's map order.
fn compose_service_names(tree: &Value) -> Vec<String> {
    match tree {
        Value::Object(map) => match map.get("services") {
            Some(Value::Object(services)) => {
                let mut names: Vec<String> = services.keys().cloned().collect();
                names.sort();
                names
            }
            _ => Vec::new(),
        },
        _ => Vec::new(),
    }
}

/// `step` runs one cross-entity pipeline step over the keyed document set: it
/// content-selects the merged compose document and PATCHes each service's
/// `environment` block, injecting an environment variable. A component-scope
/// FeatureRecipe attached to a host receives both roots; this guest acts only
/// on `host`. The host set arrives in the order the host holds it; visiting its
/// keys in sorted order, and sorting service names, keeps selection
/// deterministic across runs.
#[plugin_fn]
pub fn step(input: Vec<u8>) -> FnResult<Vec<u8>> {
    let parsed = FRPStepInput::decode_cbor(&input)?;

    let mut out = FRPStepOutput::default();

    let mut host_keys: Vec<&String> = parsed.documents.host.keys().collect();
    host_keys.sort();
    for key in host_keys {
        // An opaque unit carries no tree and is never a PATCH target. A
        // structured unit's tree rides as the bytes of its JSON document, which
        // the guest parses to select by content.
        let tree: Value = match parsed
            .documents
            .host
            .get(key)
            .and_then(|u| u.tree.as_deref())
            .and_then(|raw| serde_json::from_str(raw.get()).ok())
        {
            Some(t) => t,
            None => continue,
        };
        for svc in compose_service_names(&tree) {
            // Inject an env var into the service's `environment` map. The
            // synthesized service ships a non-empty `environment` block, so an
            // `add` of a new member is a clean node-span insertion the stock
            // codec splices in without disturbing sibling keys or styles.
            out.patches.push(JSONPatchOp {
                scope: "host".to_string(),
                key: key.clone(),
                op: "add".to_string(),
                path: format!("/services/{svc}/environment/INJECTED_BY_FEATURE"),
                value: Some(serde_json::value::to_raw_value(&Value::String(
                    "frp-step-host-patch-compose".to_string(),
                ))?),
                ..Default::default()
            });
        }
    }

    Ok(out.encode_cbor()?)
}
