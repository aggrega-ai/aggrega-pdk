// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Deliberately non-deterministic render fixture for the Aggrega Wasm runtime.
//!
//! This fixture exists to exercise the host's verify-determinism build mode: it
//! emits an output file whose bytes depend on how many times `render_node` has
//! been invoked on the instance, so two consecutive renders of identical input
//! produce different output. A guest that behaves this way is a defect — its
//! output is not a pure function of its input — and the host's render-twice
//! comparison halts the build when it detects the divergence. The fixture makes
//! that halt observable end-to-end through a real guest.
//!
//! Because its output is state-dependent, this fixture does NOT declare
//! `reusable_instance` in `describe()`: a stateful guest must never claim reuse.
//! The verify-determinism check catches the non-determinism regardless of the
//! reuse claim, because it renders twice through the same live instance.

use std::sync::atomic::{AtomicU32, Ordering};

use aggrega_pdk_rust::{RenderNodeInput, RenderNodeOutput, RenderedFile};
use base64::{engine::general_purpose::STANDARD, Engine as _};
use extism_pdk::{plugin_fn, FnResult};

/// The fixed on-disk path the fixture emits its single file at.
const OUTPUT_PATH: &str = "parity.txt";

/// The fixed prefix prepended to the node id to form the emitted file's bytes.
const CONTENT_PREFIX: &str = "rendered-node:";

/// Per-instance invocation counter. Wasm guests are single-threaded, and an
/// instance's globals persist across `render_node` calls, so this advances on
/// every crossing — the source of the deliberate non-determinism.
static COUNTER: AtomicU32 = AtomicU32::new(0);

/// The static `describe()` self-report: a v1alpha1 `provider` plugin whose
/// `render_node` export the host validates before dispatch. It declares no
/// capabilities block, so it makes no `reusable_instance` claim.
const PLUGIN_DESCRIPTOR: &str = r#"{"abi_version":"v1alpha1","plugin_type":"provider","capabilities":["parity"],"exports":[{"name":"render_node","input_pattern":"b","stability":"experimental"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}"#;

/// The host calls `describe()` once at load to read the plugin's static
/// self-report before dispatching `render_node`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(PLUGIN_DESCRIPTOR.to_string())
}

#[plugin_fn]
pub fn render_node(input: String) -> FnResult<String> {
    let parsed: RenderNodeInput = serde_json::from_str(&input)?;

    // The invocation count makes the output differ between two renders of the
    // same input — the deliberate non-determinism this fixture exists to test.
    let n = COUNTER.fetch_add(1, Ordering::Relaxed);
    let content_bytes = format!("{}{}:{}", CONTENT_PREFIX, parsed.node.id, n).into_bytes();

    let out = RenderNodeOutput {
        files: vec![RenderedFile {
            path: OUTPUT_PATH.to_string(),
            content: STANDARD.encode(&content_bytes),
        }],
        diagnostics: Vec::new(),
    };
    Ok(serde_json::to_string(&out)?)
}
