// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * The Aggrega Wasm plugin ABI version this PDK targets. The constant
 * is the `v1alpha1` contract floor; cross-language PDKs (Go, Rust,
 * TypeScript) export the same value.
 */
export const API_VERSION = "v1alpha1" as const;
