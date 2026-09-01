// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Fixture provider plugin exercising the cross-boundary edge-linking exports.
//!
//! It advertises `cross_boundary_linking` in `describe()` and offers both link
//! exports as the paired contract the host routes a cross-boundary `uses` edge
//! through:
//!
//!  - `link_publish` runs on the consumed (target) endpoint. It derives a
//!    deterministic pointer string from the edge, returns it as the published
//!    handle the consumer resolves against, and returns one structural patch
//!    that records the pointer on the consumed endpoint's assembled file.
//!  - `link_resolve` runs on the consuming (source) endpoint. It reads the
//!    published handle and returns one structural patch that injects the
//!    pointer into the consuming endpoint's assembled file.
//!
//! Each patch operation is authored as opaque JSON matching the shared pipeline
//! patch-operation wire shape (`scope`/`key`/`op`/`path`/`value`); the host
//! carries it verbatim and applies it through its codec. Output is a pure
//! function of the input, so identical input yields byte-identical output.
//!
//! The mandatory `render_node` export is present and minimal — the link pass
//! never invokes it, but a provider plugin must declare it.

use aggrega_pdk_rust::{
    encode_descriptor, InputKinds, LinkPatch, LinkPublishInput, LinkPublishOutput, LinkResolveInput,
    LinkResolveOutput, PluginDescriptor, PluginDescriptorMeta, PluginExport, ProviderContract,
    RenderNodeOutput, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};
use serde_json::value::RawValue;

/// The assembled file each endpoint's dispatch group emits and this fixture
/// wires. Both the consumed and consuming endpoints carry a file of this name
/// in their own dispatch group; the two are distinct files in distinct groups.
const LINK_FILE: &str = "boundary.yaml";

/// The static `describe()` self-report: a v1alpha1 `provider` plugin that
/// declares the `cross_boundary_linking` capability and both link exports as a
/// pair, alongside the mandatory `render_node` export.
fn descriptor() -> PluginDescriptor {
    let experimental = |name: &str| PluginExport {
        name: name.to_string(),
        input_pattern: "b".to_string(),
        stability: "experimental".to_string(),
        ..Default::default()
    };
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "provider".to_string(),
        description:
            "Wire a cross-boundary uses edge by publishing a deterministic pointer and resolving it into each endpoint's assembled file."
                .to_string(),
        capabilities: vec!["link-cross-boundary".to_string()],
        exports: vec![
            experimental("render_node"),
            experimental("link_publish"),
            experimental("link_resolve"),
        ],
        contract: Some(Box::new(ProviderContract {
            input_kinds: InputKinds {
                artifacts_standalone: true,
                ..Default::default()
            },
            boundary_assembly: "none".to_string(),
            cross_boundary_linking: true,
            ..Default::default()
        })),
        metadata: Some(Box::new(PluginDescriptorMeta {
            author: "aggrega".to_string(),
            license: "Apache-2.0".to_string(),
            ..Default::default()
        })),
        ..Default::default()
    }
}

/// derived_pointer builds the deterministic pointer string a consumed endpoint
/// publishes for one link: the target node id and the socket alias the edge is
/// bound under, joined by a fixed separator. The compiler pipes it as an opaque
/// scalar; the runtime environment resolves it at deploy time.
fn derived_pointer(target_node: &str, socket: &str) -> String {
    format!("{}--{}", target_node, socket)
}

/// one_op authors a single structural patch operation as opaque JSON matching
/// the shared pipeline patch-operation wire shape and returns it as a RawValue
/// the host carries verbatim. It addresses the file's own root by the file's
/// segmentation token (its emitted name).
fn one_op(file: &str, path: &str, value: &str) -> FnResult<Box<RawValue>> {
    let op = serde_json::json!({
        "scope": "own",
        "key": file,
        "op": "add",
        "path": path,
        "value": value,
    });
    Ok(RawValue::from_string(op.to_string())?)
}

/// The host calls `describe()` once at load to read the plugin's static
/// self-report before dispatching any export.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(encode_descriptor(&descriptor()))
}

#[plugin_fn]
pub fn render_node(_input: String) -> FnResult<String> {
    // The link pass never routes render_node; a provider plugin must still
    // declare it. Emit nothing.
    let out = RenderNodeOutput {
        files: Vec::new(),
        diagnostics: Vec::new(),
        ..Default::default()
    };
    Ok(serde_json::to_string(&out)?)
}

#[plugin_fn]
pub fn link_publish(input: String) -> FnResult<String> {
    let parsed: LinkPublishInput = serde_json::from_str(&input)?;
    let pointer = derived_pointer(&parsed.edge.target.node, &parsed.edge.socket);

    let mut handle = std::collections::BTreeMap::new();
    handle.insert("ref".to_string(), pointer.clone());

    let out = LinkPublishOutput {
        handle,
        patches: vec![LinkPatch {
            file: LINK_FILE.to_string(),
            ops: vec![one_op(LINK_FILE, "/published", &pointer)?],
        }],
        ..Default::default()
    };
    Ok(serde_json::to_string(&out)?)
}

#[plugin_fn]
pub fn link_resolve(input: String) -> FnResult<String> {
    let parsed: LinkResolveInput = serde_json::from_str(&input)?;
    // Inject the peer's published pointer verbatim into the consuming endpoint's
    // assembled file.
    let pointer = parsed.handle.get("ref").cloned().unwrap_or_default();

    let out = LinkResolveOutput {
        patches: vec![LinkPatch {
            file: LINK_FILE.to_string(),
            ops: vec![one_op(LINK_FILE, "/resolved", &pointer)?],
        }],
        ..Default::default()
    };
    Ok(serde_json::to_string(&out)?)
}
