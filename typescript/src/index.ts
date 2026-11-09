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
 * Every socket type and its encode/decode codec is generated from the
 * canonical schemas into @aggrega-ai/aggrega-abi and re-exported here,
 * single-sourcing the wire shape. The policy socket's graph is the
 * full capability-agnostic IR graph (generated from ir-v1beta1), re-exported
 * alongside the policy envelope.
 */

export * from "@aggrega-ai/aggrega-abi";
export { API_VERSION } from "./version.ts";
export { encodeDescriptor } from "./describe.ts";
export { HostError } from "./host_error.ts";
export {
  log,
  emitWarning,
  fetchExternalInput,
  packSource,
  readSourceFile,
  parseSource,
  writeStdout,
  writeStderr,
  cliArgs,
} from "./host_funcs.ts";
