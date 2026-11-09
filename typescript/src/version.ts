// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

import { AbiVersion } from "@aggrega-ai/aggrega-abi";

/**
 * The Aggrega Wasm plugin ABI version this PDK targets, re-exported from the
 * portable `@aggrega-ai/aggrega-abi` package so a plugin author writes
 * `API_VERSION` while the value itself stays single-sourced in the descriptor
 * schema that declares it. The constant is the `v1beta1` contract floor;
 * cross-language PDKs (Go, Rust, TypeScript) export the same value because all
 * three are generated from that one declaration.
 */
export const API_VERSION = AbiVersion;
