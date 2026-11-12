// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

//! Capability-label plugin for the Aggrega label-transform role.
//!
//! A capability that depends on how an instance is configured cannot be a
//! label a catalog Component carries: a PostgreSQL server is a vector store
//! only where its values enable the vector extension, and the catalog cannot
//! know whether they will. The developer declares that configuration in
//! the Aggregate's values, and the label-transform role hands each node's
//! values to the plugin attached there, so the plugin can read the
//! configuration and add the label the configuration earns.
//!
//! This plugin reads one such configuration. A node whose Component is in the
//! datastore category, and whose values hold an `extensions` list naming
//! `vector`, receives `datastore.aggrega.ai/engine: vector`. A node whose
//! values name no such extension is left as it is, and so is a node that
//! already carries an engine label: the engine label states what a product
//! is, and a product's own statement wins over a configuration's.
//!
//! The values arrive read-only. The plugin returns JSON Patch documents, and
//! the host applies them to the node labels alone, so a patch aimed at a value
//! is refused rather than applied.

use aggrega_pdk_rust::{
    encode_descriptor, MutatorInput, MutatorNode, MutatorOutput, PluginDescriptor,
    PluginDescriptorMeta, PluginExport, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};
use serde_json::{json, value::RawValue};

/// The category label and value that select the nodes the rule applies to: the
/// extension promotes a datastore, so only a datastore is considered.
const CATEGORY_LABEL: &str = "component.aggrega.ai/category";
const DATASTORE_CATEGORY: &str = "datastore";

/// The value the rule reads: a list of extension names the developer enables
/// on the instance.
const EXTENSIONS_VALUE: &str = "extensions";
/// The extension whose presence promotes the label.
const VECTOR_EXTENSION: &str = "vector";

/// The capability label the rule adds.
const ENGINE_LABEL: &str = "datastore.aggrega.ai/engine";
const VECTOR_ENGINE: &str = "vector";

/// The static `describe()` self-report: a governance plugin whose `mutate`
/// export the host validates before dispatch at the label-transform role.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "governance".to_string(),
        description: "Label a datastore whose values enable the vector extension as a vector engine."
            .to_string(),
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

/// Reports whether a node's labels and values earn the vector engine label:
/// the node is a datastore, carries no engine label of its own, and its values
/// hold an extensions list naming the vector extension. A value of another
/// shape under that key is not a list of extensions, so it earns nothing
/// rather than failing the build.
fn promotes(node: &MutatorNode) -> bool {
    if node.labels.get(CATEGORY_LABEL).map(String::as_str) != Some(DATASTORE_CATEGORY) {
        return false;
    }
    if node.labels.get(ENGINE_LABEL).is_some() {
        return false;
    }
    let Some(raw) = node.values.get(EXTENSIONS_VALUE) else {
        return false;
    };
    match serde_json::from_str::<Vec<String>>(raw.get()) {
        Ok(extensions) => extensions.iter().any(|e| e == VECTOR_EXTENSION),
        Err(_) => false,
    }
}

/// Escapes a JSON Pointer reference token per RFC 6901: `~` becomes `~0` and
/// `/` becomes `~1`, in that order, so a node ID or a label key carrying a `/`
/// addresses one entry rather than a nested path.
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

    // Sorted node IDs make the patch list independent of the order the nodes
    // arrive in, so identical input yields byte-identical output.
    let mut node_ids: Vec<&String> = parsed.graph.nodes.keys().collect();
    node_ids.sort();

    let mut patches: Vec<Box<RawValue>> = Vec::new();
    for node_id in node_ids {
        let Some(node) = parsed.graph.nodes.get(node_id) else {
            continue;
        };
        if !promotes(node) {
            continue;
        }
        let doc = json!([{
            "op": "add",
            "path": format!(
                "/nodes/{}/labels/{}",
                escape_pointer_token(node_id),
                escape_pointer_token(ENGINE_LABEL)
            ),
            "value": VECTOR_ENGINE,
        }]);
        patches.push(serde_json::value::to_raw_value(&doc)?);
    }

    let out = MutatorOutput {
        patches,
        ..Default::default()
    };
    Ok(out.encode_cbor()?)
}
