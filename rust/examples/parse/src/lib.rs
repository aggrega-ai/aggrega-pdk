// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

//! Reference parser plugin for the Aggrega Wasm runtime.
//!
//! A parser turns a foreign source text into a canonical structured tree, so
//! any consumer can read the tree without carrying its own language parser.
//! This example is a minimal scaffold rather than a real language parser: it
//! self-describes as a parser plugin advertising the `parse:demo` format, and
//! its `parse` export echoes the input it receives back as a small structured
//! tree. The point is to exercise both directions of the generated parse codec
//! — the export decodes a `ParseInput` and encodes a `ParseOutput` — not to
//! implement a grammar.
//!
//! The host reads `describe()` once at load, validates the descriptor, then
//! dispatches `parse` with a `ParseInput` as input.
//!
//! The parser socket carries canonical CBOR in both directions, so the export
//! takes and returns bytes, reading the input with `ParseInput::decode_cbor` and
//! writing the output with `ParseOutput::encode_cbor`. The tree the output
//! carries is a JSON document, and stays one: it crosses as a byte string holding
//! that document verbatim.

use aggrega_pdk_rust::{
    encode_descriptor, ParseInput, ParseOutput, PluginDescriptor, PluginDescriptorMeta,
    PluginExport, API_VERSION,
};
use base64::Engine;
use extism_pdk::{plugin_fn, FnResult};
use serde_json::json;
use serde_json::value::to_raw_value;

/// The static `describe()` self-report: a parser plugin whose `parse`
/// export handles the `parse:demo` format.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "parser".to_string(),
        description: "Echo a source text back as a small structured tree — a minimal parser scaffold."
            .to_string(),
        capabilities: vec!["parse:demo".to_string()],
        exports: vec![PluginExport {
            name: "parse".to_string(),
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
/// self-declaration before dispatching `parse`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(encode_descriptor(&descriptor()))
}

/// `parse` decodes a `ParseInput` and returns a `ParseOutput`. A real parser
/// would read the bytes as the named format and build the canonical tree; this
/// scaffold echoes the input as a small object tree so the example exercises
/// both codec directions without a grammar.
#[plugin_fn]
pub fn parse(input: Vec<u8>) -> FnResult<Vec<u8>> {
    let parsed = ParseInput::decode_cbor(&input)?;

    // The source bytes cross the wire as a raw byte string, and this kit's
    // `bytes` field holds them base64-encoded — the generated decoder does that
    // conversion — so decode them to recover the original text for the echoed
    // tree. A decode failure falls back to the empty string rather than failing
    // the parse — this is a scaffold, not a grammar.
    let source = base64::engine::general_purpose::STANDARD
        .decode(parsed.bytes.as_bytes())
        .ok()
        .and_then(|b| String::from_utf8(b).ok())
        .unwrap_or_default();

    let tree = to_raw_value(&json!({
        "format": parsed.format,
        "source": source,
    }))?;

    let out = ParseOutput {
        tree,
        diagnostics: Vec::new(),
        spans: Vec::new(),
        error: String::new(),
    };

    Ok(out.encode_cbor()?)
}
