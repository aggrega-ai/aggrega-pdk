// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Aggrega Plugin Development Kit for Rust.
//!
//! Plugin authors targeting the Aggrega Wasm plugin ABI consume this
//! crate to obtain typed socket payloads, the [`APIVersion`] contract
//! floor constant, and bindings to the Aggrega host functions
//! (`aggrega_log`, `aggrega_emit_warning`).
//!
//! Build a plugin to a `.wasm` artifact via:
//!
//! ```text
//! cargo build --release --target wasm32-unknown-unknown
//! ```
//!
//! See the crate-level README and the per-fixture examples under
//! `examples/` for working plugin sources.

pub mod host_funcs;
pub mod types;
pub mod version;

pub use host_funcs::{emit_warning, log};
pub use types::{
    CompilationContext, MutatorGraph, MutatorInput, MutatorNode, MutatorOutput, PolicyEdge,
    PolicyFeatureEntry, PolicyGraph, PolicyNode, PolicyResult, PolicyValidatorInput,
    PolicyValidatorOutput, PrecalculatedPath,
};
pub use version::API_VERSION;
