// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Fixture provider plugin for the Aggrega Wasm runtime's render socket.
//!
//! Applies one trivial, deterministic transform — emits a single output
//! file at a fixed path whose bytes are `rendered-node:` followed by the
//! node id — and returns it as the sole [`RenderedFile`] of a
//! [`RenderNodeOutput`]. The transform is identical across the per-language
//! render fixtures, so the same [`RenderNodeInput`] yields equivalent output
//! whichever language rendered it. The host verifies end-to-end that:
//!
//!  - The plugin loaded successfully via the Aggrega Wasm loader.
//!  - Input JSON deserialised to [`RenderNodeInput`].
//!  - The render transform ran against the node projection.
//!  - Output JSON serialises to a [`RenderNodeOutput`] whose file content
//!    round-trips through the render socket's base64 wire encoding.

use aggrega_pdk_rust::{
    encode_descriptor, PluginDescriptor, PluginDescriptorMeta, PluginExport, RenderNodeInput,
    RenderNodeOutput, RenderedFile, API_VERSION,
};
use base64::{engine::general_purpose::STANDARD, Engine as _};
use extism_pdk::{plugin_fn, FnResult};

/// The fixed on-disk path the fixture emits its single file at. Constant
/// across the per-language render fixtures so their outputs compare equal.
const OUTPUT_PATH: &str = "parity.txt";

/// The fixed prefix prepended to the node id to form the emitted file's
/// bytes. Constant across the per-language render fixtures so identical
/// input produces identical content.
const CONTENT_PREFIX: &str = "rendered-node:";

/// The static `describe()` self-report: a v1alpha1 `provider` plugin whose
/// `render_node` export the host validates before dispatch.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "provider".to_string(),
        description:
            "Render one IR node to a single output file whose bytes are the node id under a fixed prefix."
                .to_string(),
        capabilities: vec!["parity".to_string()],
        exports: vec![PluginExport {
            name: "render_node".to_string(),
            input_pattern: "b".to_string(),
            stability: "experimental".to_string(),
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

/// The host calls `describe()` once at load to read the plugin's static
/// self-report before dispatching `render_node`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(encode_descriptor(&descriptor()))
}

#[plugin_fn]
pub fn render_node(input: String) -> FnResult<String> {
    let parsed: RenderNodeInput = serde_json::from_str(&input)?;

    // The file content is a fixed function of the node id. The render
    // socket carries file content as base64 on the JSON wire, so the
    // guest base64-encodes the raw bytes; the host decodes them verbatim.
    let content_bytes = format!("{}{}", CONTENT_PREFIX, parsed.node.id).into_bytes();

    let out = RenderNodeOutput {
        files: vec![RenderedFile {
            path: OUTPUT_PATH.to_string(),
            content: STANDARD.encode(&content_bytes),
        }],
        diagnostics: Vec::new(),
    };
    Ok(serde_json::to_string(&out)?)
}
