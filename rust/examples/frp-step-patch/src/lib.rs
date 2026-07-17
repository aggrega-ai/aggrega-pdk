// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Reference feature-recipe-pipeline step plugin for the Aggrega Wasm
//! runtime.
//!
//! Unlike the no-op `frp-step` fixture, this plugin exercises the real
//! pipeline contract end-to-end:
//!
//!  - It content-selects a workload from the keyed document set (a
//!    document whose `kind` is `Deployment`), learns that document's key
//!    at runtime, and roots an own-scope PATCH at it — adding an
//!    annotation.
//!  - It EMITs a new own-scope ConfigMap document (a pipeline side
//!    effect).
//!  - It EMITs a new host-scope marker document (an additive cross-entity
//!    side effect that runs per-node for every provider).
//!
//! It self-describes via `describe()` as an `frp_step` plugin whose
//! `step` export declares `operates=patch`, `scope=own`, and a helm
//! compatible provider. The host reads `describe()` once at load,
//! validates the descriptor, then dispatches `step` with the keyed
//! document set as input.

use aggrega_pdk_rust::{
    encode_descriptor, PluginDescriptor, PluginDescriptorMeta, PluginExport, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::BTreeMap;

/// The static `describe()` self-report: a v1alpha1 `frp_step` plugin
/// whose `step` export patches own-scope workloads and is
/// tested-compatible with the helm and helm-chart providers (both render
/// workloads carrying a pod template, in flat-document and chart-source form
/// respectively).
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "frp_step".to_string(),
        description:
            "Patch a selected workload and emit companion documents in one feature-recipe pipeline step."
                .to_string(),
        capabilities: vec!["helm".to_string(), "helm-chart".to_string()],
        exports: vec![PluginExport {
            name: "step".to_string(),
            operates: "patch".to_string(),
            scope: "own".to_string(),
            selector: "workloads carrying a pod template".to_string(),
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

/// One addressable unit the host hands the guest: the codec's hybrid view of
/// one piece of output. A structured unit carries a `tree` (the
/// RFC-6902-addressable view the guest content-selects and patches); an
/// opaque unit carries only its `media_type` and bytes and no `tree`, so it
/// is a SELECT/EMIT target the guest never patches. The token and bytes ride
/// the wire but this plugin reads only the tree.
#[derive(Deserialize)]
struct Unit {
    #[serde(default)]
    tree: Option<Value>,
}

/// The keyed, scope-rooted unit set the host hands the guest. The guest reads
/// each unit's content to select, never a filename. The host root is present
/// only for a component-scope FeatureRecipe attached to a host; the guest
/// checks it before emitting a host-scope side effect.
#[derive(Deserialize)]
struct DocumentSet {
    #[serde(default)]
    own: BTreeMap<String, Unit>,
    #[serde(default)]
    host: BTreeMap<String, Unit>,
}

/// The subset of the host's FRPStepInput this plugin reads. The step
/// input, render context, and preceding steps ride on the wire but are
/// unused here.
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

/// The file an `emit_file` side effect writes.
#[derive(Serialize)]
struct SideEffectFile {
    path: String,
    content: String,
}

/// One pipeline side effect (an additive EMIT).
#[derive(Serialize)]
struct SideEffect {
    kind: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    scope: String,
    file: SideEffectFile,
}

/// The host's FRPStepOutput wire shape: key-addressed patches plus
/// additive side effects.
#[derive(Serialize, Default)]
struct StepOutput {
    #[serde(skip_serializing_if = "Vec::is_empty")]
    patches: Vec<JsonPatchOp>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    side_effects: Vec<SideEffect>,
}

/// The host calls `describe()` once at load to read the plugin's
/// self-declaration before dispatching `step`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(encode_descriptor(&descriptor()))
}

/// `step` runs one pipeline step over the keyed document set: it
/// content-selects a Deployment, PATCHes it (own-scope annotation), and
/// EMITs an own-scope ConfigMap plus a host-scope marker.
#[plugin_fn]
pub fn step(input: String) -> FnResult<String> {
    let parsed: StepInput = serde_json::from_str(&input)?;

    let mut out = StepOutput::default();

    // Content-select a Deployment in the own set and annotate it. The guest
    // learns the unit's key at runtime — it never knows emitted filenames
    // ahead of time — and roots its patch at that key. It reads the unit's
    // tree (skipping opaque units that carry none) and inspects content.
    // Iterate a BTreeMap so selection is deterministic across runs.
    for (key, unit) in parsed.documents.own.iter() {
        let kind = unit
            .tree
            .as_ref()
            .and_then(|t| t.get("kind"))
            .and_then(Value::as_str);
        if kind == Some("Deployment") {
            out.patches.push(JsonPatchOp {
                scope: "own".to_string(),
                key: key.clone(),
                op: "add".to_string(),
                // RFC 6901 escapes `/` in the annotation key as `~1`.
                path: "/metadata/annotations/aggrega.ai~1pipeline-patched".to_string(),
                value: Value::String("true".to_string()),
            });
        }
    }

    // Own-scope EMIT: a new ConfigMap document the pipeline contributes.
    out.side_effects.push(SideEffect {
        kind: "emit_file".to_string(),
        scope: "own".to_string(),
        file: SideEffectFile {
            path: "pipeline-configmap.yaml".to_string(),
            content:
                "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: pipeline-emitted\ndata:\n  source: frp-step-patch\n"
                    .to_string(),
        },
    });

    // Host-scope EMIT: an additive marker the pipeline contributes to the
    // host component's set — only when a host set is present (a component-
    // scope FeatureRecipe attached to a host). A Recipe step has no host
    // scope, so the guest does not emit one. Additive emits run per-node for
    // every provider.
    if !parsed.documents.host.is_empty() {
        out.side_effects.push(SideEffect {
            kind: "emit_file".to_string(),
            scope: "host".to_string(),
            file: SideEffectFile {
                path: "pipeline-host-marker.yaml".to_string(),
                content:
                    "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: pipeline-host-marker\ndata:\n  injected-by: frp-step-patch\n"
                        .to_string(),
            },
        });
    }

    Ok(serde_json::to_string(&out)?)
}
