// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * @aggrega-ai/aggrega-pdk-typescript
 *
 * Plugin Development Kit for authoring Aggrega Wasm plugins in
 * TypeScript. Re-exports the socket payload types and codecs, the
 * API_VERSION contract floor constant, and the safe wrappers around the
 * Aggrega host functions.
 *
 * The mutator and linter socket types and their encode/decode codecs are
 * generated from the canonical schemas into @aggrega-ai/aggrega-abi and
 * re-exported here, single-sourcing the wire shape. The policy-validator
 * socket types remain hand-authored in ./types.ts pending their own
 * schema-derived collapse.
 */

export * from "@aggrega-ai/aggrega-abi";
export * from "./types.ts";
export { API_VERSION } from "./version.ts";
export { log, emitWarning } from "./host_funcs.ts";
