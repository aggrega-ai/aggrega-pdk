// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Fixture provider plugin for the Aggrega Wasm runtime's render socket.
//!
//! Applies two trivial, deterministic transforms and returns each as a
//! [`RenderedFile`] of a [`RenderNodeOutput`]: one file whose bytes are
//! `rendered-node:` followed by the node id, and one reporting the node's
//! attachments as `alias=sensitive|plain` in the order this guest observes them.
//! The first transform is identical across the per-language render fixtures, so
//! the same [`RenderNodeInput`] yields the same node-id echo whichever language
//! rendered it. The second is deliberately allowed to differ: it reports what
//! this language's generated codec does with an author-ordered map, which is a
//! property of the language's own map container rather than of the socket.
//! Beside the files it reports two [`RenderFact`]s about what it emitted, in a
//! fixed order that is deliberately not name order: the paths it wrote, and the
//! node it echoed. The host verifies end-to-end that:
//!
//!  - The plugin loaded successfully via the Aggrega Wasm loader.
//!  - Input JSON deserialised to [`RenderNodeInput`].
//!  - The render transform ran against the node projection.
//!  - An attachment's sensitivity marking is readable from the payload alone.
//!  - Output JSON serialises to a [`RenderNodeOutput`] whose file content
//!    round-trips through the render socket's base64 wire encoding.
//!  - The reported facts reach the host in the order listed here, each value
//!    byte for byte as written here.

use aggrega_pdk_rust::{
    encode_descriptor, IRFeatureEntry, PluginDescriptor, PluginDescriptorMeta, PluginExport,
    RenderFact, RenderNodeInput, RenderNodeOutput, RenderedFile, API_VERSION,
};
use base64::{engine::general_purpose::STANDARD, Engine as _};
use extism_pdk::{plugin_fn, FnResult};
use serde_json::value::RawValue;

/// The fixed on-disk path the fixture emits its node-id echo at. Constant
/// across the per-language render fixtures so their outputs compare equal.
const OUTPUT_PATH: &str = "parity.txt";

/// The fixed on-disk path the fixture emits its attachment echo at. Constant
/// across the per-language render fixtures; the CONTENT is deliberately allowed
/// to differ per language, because it reports what this language's generated
/// codec actually did with an author-ordered attachment map.
const ATTACHMENTS_PATH: &str = "attachments.txt";

/// The fixed prefix prepended to the node id to form the emitted file's
/// bytes. Constant across the per-language render fixtures so identical
/// input produces identical content.
const CONTENT_PREFIX: &str = "rendered-node:";

/// The format the fixture declares its emitted file was rendered in, so the
/// host can route the file to a matching parser. A provider that declares no
/// format leaves the field unset and the host infers it from the extension.
const OUTPUT_FORMAT: &str = "text";

/// The static `describe()` self-report: a v1alpha1 `provider` plugin whose
/// `render_node` export the host validates before dispatch.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "provider".to_string(),
        description:
            "Render one IR node to a node-id echo file and an attachment-report file."
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

/// Reports the node's attachments as `alias=sensitive|plain`, comma-joined, in
/// the order this guest observes them. The attachment entry arrives opaque at the
/// render socket's own layer, so it is decoded through the IR wire contract's
/// feature-entry shape and the marking is read from that shape's `sensitive`
/// field rather than guessed from the alias.
///
/// What that field means is worth stating precisely, because a guest that reads
/// it as more than this marks values that are not secret:
///
///  - **It describes the ATTACHMENT, never one datum.** It is true when the
///    attachment handles material declared sensitive *anywhere*, and says
///    nothing about the specific value any given variable in the render context
///    carries. A guest that emitted a target-native sensitivity primitive per
///    rendered value on the strength of this field would mark values that are
///    not secret.
///  - **The host derives it from either of two independent declarations:** the
///    attached Feature declaring a sensitive leaf in its `values`, `infra` or
///    `provides` pillar, or the host Component's attachment slot of the same
///    name declaring that whatever binds there handles secret material. Those
///    are different claims — a leaf says *this datum is secret material*, a slot
///    says *whatever binds here handles secret material* — and either alone is
///    enough.
///  - **The wire field carries that derived answer; it does not declare it.**
///    The declarations live in the catalog entities the host read, so a guest
///    cannot tell from this field alone which of the two fired, or which leaf
///    carried the marking.
///
/// The field is omitted from the payload when the derivation produced false, so
/// a failed decode and an absent field both settle to `plain` here.
fn attachment_echo(parsed: &RenderNodeInput) -> String {
    parsed
        .node
        .features
        .iter()
        .map(|(alias, raw)| {
            let sensitive = serde_json::from_str::<IRFeatureEntry>(raw.get())
                .map(|entry| entry.sensitive)
                .unwrap_or(false);
            format!(
                "{}={}",
                alias,
                if sensitive { "sensitive" } else { "plain" }
            )
        })
        .collect::<Vec<_>>()
        .join(",")
}

/// The facts this fixture reports about what it emitted, identical across the
/// per-language render fixtures so the host can compare them byte for byte.
///
/// A fact is a name and a list of entries, and both are this provider's own
/// vocabulary: the host carries them in the order listed, refuses a value that is
/// not a list, and learns nothing else about either. Every value is a list even
/// with one entry, because the host joins the lists one name gets from every
/// render of a node. The value is the JSON this guest writes, so it is built here
/// as text rather than through a map type that would re-order its keys — `prefix`
/// deliberately precedes `id`.
fn render_facts(node_id: &str) -> Result<Vec<RenderFact>, serde_json::Error> {
    let emitted_paths = serde_json::to_string(&[OUTPUT_PATH, ATTACHMENTS_PATH])?;
    let echoed_node = format!(
        "[{{\"prefix\":{},\"id\":{}}}]",
        serde_json::to_string(CONTENT_PREFIX)?,
        serde_json::to_string(node_id)?
    );
    Ok(vec![
        RenderFact {
            name: "emitted_paths".to_string(),
            value: RawValue::from_string(emitted_paths)?,
        },
        RenderFact {
            name: "echoed_node".to_string(),
            value: RawValue::from_string(echoed_node)?,
        },
    ])
}

#[plugin_fn]
pub fn render_node(input: String) -> FnResult<String> {
    let parsed: RenderNodeInput = serde_json::from_str(&input)?;

    // The file content is a fixed function of the node id. The render
    // socket carries file content as base64 on the JSON wire, so the
    // guest base64-encodes the raw bytes; the host decodes them verbatim.
    let content_bytes = format!("{}{}", CONTENT_PREFIX, parsed.node.id).into_bytes();
    let attachment_bytes = attachment_echo(&parsed).into_bytes();

    let out = RenderNodeOutput {
        files: vec![
            RenderedFile {
                path: OUTPUT_PATH.to_string(),
                content: STANDARD.encode(&content_bytes),
                format: OUTPUT_FORMAT.to_string(),
            },
            RenderedFile {
                path: ATTACHMENTS_PATH.to_string(),
                content: STANDARD.encode(&attachment_bytes),
                format: OUTPUT_FORMAT.to_string(),
            },
        ],
        render_facts: render_facts(&parsed.node.id)?,
        diagnostics: Vec::new(),
        ..Default::default()
    };
    Ok(serde_json::to_string(&out)?)
}
