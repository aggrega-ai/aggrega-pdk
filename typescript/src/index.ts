// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * @aggrega-ai/aggrega-pdk-typescript
 *
 * Plugin Development Kit for authoring Aggrega Wasm plugins in
 * TypeScript. Re-exports the socket payload types, the API_VERSION
 * contract floor constant, and the safe wrappers around the Aggrega
 * host functions.
 */

export * from "./types.ts";
export { API_VERSION } from "./version.ts";
export { log, emitWarning } from "./host_funcs.ts";
