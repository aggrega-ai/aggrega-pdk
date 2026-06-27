// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Fixture mutator plugin for the Aggrega Wasm runtime.
//!
//! Applies one trivial transform — appends the label
//! `fixture.aggrega-pdk/touched = mutator-rust` to every node in the
//! [`MutatorGraph`] — and returns one JSON Patch document per node
//! carrying that addition. The host verifies end-to-end that:
//!
//!  - The plugin loaded successfully via the Aggrega Wasm loader.
//!  - Input JSON deserialised to [`MutatorInput`].
//!  - The transform applied per node.
//!  - Output JSON serialises to a [`MutatorOutput`] valid against the
//!    `wasm-mutator-v1alpha1` JSON Schema.

use aggrega_pdk_rust::{MutatorInput, MutatorOutput};
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

#[plugin_fn]
pub fn mutate(input: String) -> FnResult<String> {
    let parsed: MutatorInput = serde_json::from_str(&input)?;

    // Iterate node IDs in sorted order. `HashMap` iteration is
    // non-deterministic; sorting keeps the emitted patches
    // byte-identical across runs of identical input and matches the
    // Aggrega project's determinism discipline.
    let mut node_ids: Vec<&String> = parsed.graph.nodes.keys().collect();
    node_ids.sort();

    // The mutator socket's patches are opaque JSON Patch documents; the abi
    // MutatorOutput carries them as verbatim RawValue, so each json! value is
    // converted to a RawValue.
    let patches: Vec<Box<RawValue>> = node_ids
        .into_iter()
        .map(|node_id| {
            let doc = json!([{
                "op": "add",
                "path": format!("/nodes/{}/labels/{}", node_id, FIXTURE_LABEL_KEY),
                "value": FIXTURE_LABEL_VALUE,
            }]);
            serde_json::value::to_raw_value(&doc).expect("encode patch")
        })
        .collect();

    let out = MutatorOutput { patches };
    Ok(serde_json::to_string(&out)?)
}
