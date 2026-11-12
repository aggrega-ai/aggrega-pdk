// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

/// The Aggrega Wasm plugin ABI version this PDK targets, re-exported from the
/// portable `aggrega-abi` crate so a plugin author writes `API_VERSION` while
/// the value itself stays single-sourced in the descriptor schema that declares
/// it. The constant is the `v1beta1` contract floor; cross-language PDKs (Go,
/// Rust, TypeScript) export the same value because all three are generated from
/// that one declaration.
pub const API_VERSION: &str = aggrega_abi::ABI_VERSION;
