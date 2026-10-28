// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Fixture governance plugin for the `label-transform` role, for the Aggrega Wasm runtime.
//!
//! Applies one trivial transform — appends the label
//! `fixture.aggrega-pdk/touched = mutator-rust` to every node in the
//! mutation graph — and returns one JSON Patch document per node
//! carrying that addition. The host verifies end-to-end that:
//!
//!  - The plugin loaded successfully via the Aggrega Wasm loader.
//!  - The canonical CBOR payload decoded to [`MutatorInput`].
//!  - The transform applied per node.
//!  - The result encoded to a [`MutatorOutput`] valid against the
//!    `wasm-mutator-v1alpha1` schema.
//!
//! The label-transform role's wire carries canonical CBOR in both directions, so the
//! export takes and returns bytes. The patch documents the output carries are
//! JSON, and stay JSON: the socket carries them verbatim and neither side reads
//! into them.

use aggrega_pdk_rust::{
    encode_descriptor, MutatorInput, MutatorOutput, PluginDescriptor, PluginDescriptorMeta,
    PluginExport, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};
use serde_json::{json, value::RawValue};

/// Label key the fixture appends to every node. The key identifies
/// the source of the label as a fixture plugin so end-to-end tests on
/// the host can recognise the transform.
const FIXTURE_LABEL_KEY: &str = "fixture.aggrega-pdk/touched";

/// Label value the fixture writes. Identifies the source language so
/// cross-language fixture-parity tests can distinguish per-language
/// outputs.
const FIXTURE_LABEL_VALUE: &str = "mutator-rust";

/// The static `describe()` self-report: a v1alpha1 `governance` plugin
/// whose `mutate` export the host validates before dispatch.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "governance".to_string(),
        description: "Append a fixed label to every node in the mutation graph.".to_string(),
        exports: vec![PluginExport {
            name: "mutate".to_string(),
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

/// Escapes a JSON Pointer reference token per RFC 6901: `~` becomes `~0`
/// and `/` becomes `~1`. The order matters — `~` is escaped first so a
/// `/` rewritten to `~1` is not re-escaped. The label key carries a `/`,
/// so escaping is required for the emitted patch path to address a single
/// label entry rather than a non-existent nested path.
fn escape_pointer_token(tok: &str) -> String {
    tok.replace('~', "~0").replace('/', "~1")
}

/// The host calls `describe()` once at load to read the plugin's static
/// self-report before dispatching `mutate`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(encode_descriptor(&descriptor()))
}

#[plugin_fn]
pub fn mutate(input: Vec<u8>) -> FnResult<Vec<u8>> {
    let parsed = MutatorInput::decode_cbor(&input)?;

    // Attachment inputs arrive alongside the graph. `settings` is a free-form
    // object the host passes through verbatim; the optional "label_value" key
    // overrides the label the fixture writes, defaulting to the language
    // constant when no settings are attached. `sources` carry the declared
    // definition files as { path, content } — a real mutator parses them to
    // drive its labelling; the fixture simply reads them to demonstrate access.
    // Both are absent unless the attachment declared a `with:` block.
    let mut label_value = FIXTURE_LABEL_VALUE.to_string();
    if let Some(raw) = &parsed.settings {
        if let Ok(s) = serde_json::from_str::<serde_json::Value>(raw.get()) {
            if let Some(v) = s.get("label_value").and_then(|v| v.as_str()) {
                label_value = v.to_string();
            }
        }
    }
    for source in &parsed.sources {
        // A real plugin parses source.content (addressed by source.path) here.
        let _ = source;
    }

    // Iterate node IDs in sorted order. The nodes arrive in an `OrderedMap`,
    // in the order the host sent them; sorting makes the emitted patches
    // independent of that order, so identical input yields byte-identical
    // output, matching the Aggrega project's determinism discipline.
    let mut node_ids: Vec<&String> = parsed.graph.nodes.keys().collect();
    node_ids.sort();

    // The label-transform role's patches are opaque JSON Patch documents; the abi
    // MutatorOutput carries them as verbatim RawValue, so each json! value is
    // converted to a RawValue.
    let patches: Vec<Box<RawValue>> = node_ids
        .into_iter()
        .map(|node_id| {
            let doc = json!([{
                "op": "add",
                "path": format!(
                    "/nodes/{}/labels/{}",
                    escape_pointer_token(node_id),
                    escape_pointer_token(FIXTURE_LABEL_KEY)
                ),
                "value": label_value,
            }]);
            serde_json::value::to_raw_value(&doc).expect("encode patch")
        })
        .collect();

    let out = MutatorOutput {
        patches,
        ..Default::default()
    };
    Ok(out.encode_cbor()?)
}

#[cfg(test)]
mod tests {
    use super::escape_pointer_token;

    #[test]
    fn escapes_slash_to_tilde_one() {
        assert_eq!(
            escape_pointer_token("fixture.aggrega-pdk/touched"),
            "fixture.aggrega-pdk~1touched"
        );
    }

    #[test]
    fn escapes_tilde_to_tilde_zero() {
        assert_eq!(escape_pointer_token("a~b"), "a~0b");
    }

    #[test]
    fn escapes_tilde_before_slash() {
        // "~" is escaped first, so a "/" rewritten to "~1" is not re-escaped.
        assert_eq!(escape_pointer_token("~/"), "~0~1");
    }
}
