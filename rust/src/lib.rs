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

// The mutator and linter socket types are generated from the canonical schemas
// into the portable aggrega-abi crate and re-exported here, so plugin authors
// keep the flat `aggrega_pdk_rust::<Type>` spelling while the wire shape stays
// single-sourced in aggrega-abi.
pub use aggrega_abi::{
    CompilationContext, LinterFinding, LinterInput, LinterOutput, MutatorGraph, MutatorInput,
    MutatorNode, MutatorOutput, NarrowingExemption, Policies,
};

// The policy-validator socket types remain hand-authored pending their own
// schema-derived collapse.
pub use types::{
    PolicyEdge, PolicyFeatureEntry, PolicyGraph, PolicyNode, PolicyResult, PolicyValidatorInput,
    PolicyValidatorOutput, PrecalculatedPath,
};
pub use version::API_VERSION;
