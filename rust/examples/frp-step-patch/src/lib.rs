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
//! It self-describes via `describe()` as a `pipeline-step` plugin whose
//! `step` export declares `operates=patch`, `scope=own`, and a helm
//! compatible provider. The host reads `describe()` once at load,
//! validates the descriptor, then dispatches `step` with the keyed
//! document set as input.
//!
//! The pipeline-step socket carries canonical CBOR in both directions, so
//! `step` takes and returns bytes: it reads its input with
//! `FRPStepInput::decode_cbor` and returns its output's `encode_cbor()`,
//! through the codec generated into the `aggrega-abi` crate.

use aggrega_abi::{FRPStepInput, FRPStepOutput, JSONPatchOp, SideEffect, SideEffectFile};
use aggrega_pdk_rust::{
    encode_descriptor, PluginDescriptor, PluginDescriptorMeta, PluginExport, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};
use serde_json::Value;

/// The static `describe()` self-report: a `pipeline-step` plugin
/// whose `step` export patches own-scope workloads and is
/// tested-compatible with the helm and helm-chart providers (both render
/// workloads carrying a pod template, in flat-document and chart-source form
/// respectively).
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "pipeline-step".to_string(),
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

/// The `kind` of a unit's tree, or `None` when the unit is opaque (no tree:
/// a SELECT/EMIT target the guest never patches) or its tree carries no kind.
/// The tree rides as the bytes of its JSON document, which the guest parses.
fn tree_kind(tree: Option<&serde_json::value::RawValue>) -> Option<String> {
    let doc: Value = serde_json::from_str(tree?.get()).ok()?;
    doc.get("kind")?.as_str().map(str::to_string)
}

/// One additive EMIT of a whole file into the named scope.
fn emit_file(scope: &str, path: &str, content: &str) -> SideEffect {
    SideEffect {
        kind: "emit_file".to_string(),
        scope: scope.to_string(),
        file: Some(Box::new(SideEffectFile {
            path: path.to_string(),
            content: content.to_string(),
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

/// `step` runs one pipeline step over the keyed document set: it
/// content-selects a Deployment, PATCHes it (own-scope annotation), and
/// EMITs an own-scope ConfigMap plus a host-scope marker.
#[plugin_fn]
pub fn step(input: Vec<u8>) -> FnResult<Vec<u8>> {
    let parsed = FRPStepInput::decode_cbor(&input)?;

    let mut out = FRPStepOutput::default();

    // Content-select a Deployment in the own set and annotate it. The guest
    // learns the unit's key at runtime — it never knows emitted filenames
    // ahead of time — and roots its patch at that key. It reads the unit's
    // tree (skipping opaque units that carry none) and inspects content. The
    // own set arrives in the order the host holds it; visit its keys in sorted
    // order so selection is deterministic across runs.
    let mut own_keys: Vec<&String> = parsed.documents.own.keys().collect();
    own_keys.sort();
    for key in own_keys {
        let unit = parsed.documents.own.get(key);
        if tree_kind(unit.and_then(|u| u.tree.as_deref())).as_deref() == Some("Deployment") {
            out.patches.push(JSONPatchOp {
                scope: "own".to_string(),
                key: key.clone(),
                op: "add".to_string(),
                // RFC 6901 escapes `/` in the annotation key as `~1`.
                path: "/metadata/annotations/aggrega.ai~1pipeline-patched".to_string(),
                value: Some(serde_json::value::to_raw_value(&Value::String("true".to_string()))?),
                ..Default::default()
            });
        }
    }

    // Own-scope EMIT: a new ConfigMap document the pipeline contributes.
    out.side_effects.push(emit_file(
        "own",
        "pipeline-configmap.yaml",
        "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: pipeline-emitted\ndata:\n  source: frp-step-patch\n",
    ));

    // Host-scope EMIT: an additive marker the pipeline contributes to the
    // host component's set — only when a host set is present (a component-
    // scope FeatureRecipe attached to a host). A Recipe step has no host
    // scope, so the guest does not emit one. Additive emits run per-node for
    // every provider.
    if !parsed.documents.host.is_empty() {
        out.side_effects.push(emit_file(
            "host",
            "pipeline-host-marker.yaml",
            "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: pipeline-host-marker\ndata:\n  injected-by: frp-step-patch\n",
        ));
    }

    Ok(out.encode_cbor()?)
}
