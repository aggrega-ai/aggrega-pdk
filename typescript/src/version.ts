// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * The Aggrega Wasm plugin ABI version this PDK targets. The constant
 * is the `v1alpha1` contract floor; cross-language PDKs (Go, Rust,
 * TypeScript) export the same value.
 */
export const API_VERSION = "v1alpha1" as const;

/**
 * Build-time provenance constants. They do not exist at runtime unless
 * the plugin's esbuild bundle step injects them with `--define`:
 *
 * ```sh
 * npx esbuild src/index.ts --bundle --format=cjs --target=es2022 \
 *   --platform=neutral --outfile=dist/plugin.js \
 *   --define:AGGREGA_PLUGIN_VERSION='"1.2.3"' \
 *   --define:AGGREGA_PLUGIN_COMMIT='"abc1234"'
 * ```
 *
 * They are declared (not defined) here so the type-checker accepts the
 * references; {@link provenanceMetadata} reads them behind a `typeof`
 * guard so a plain, undefined build yields empty rather than a
 * `ReferenceError`.
 */
declare const AGGREGA_PLUGIN_VERSION: string | undefined;
declare const AGGREGA_PLUGIN_COMMIT: string | undefined;

/**
 * Returns the optional build-provenance fragment a plugin appends inside
 * its `describe()` descriptor's `"metadata"` object. Each defined
 * constant contributes a leading-comma JSON member — for example
 * `,"version":"1.2.3","commit":"abc1234"` — so the fragment drops
 * straight in after the fixed metadata members:
 *
 * ```ts
 * const head = `{"abi_version":"v1alpha1","metadata":{"author":"me"`;
 * const descriptor = `${head}${provenanceMetadata()}}}`;
 * ```
 *
 * When esbuild does not `--define` the constants the fragment is empty,
 * keeping an unstamped descriptor byte-identical to a stamped one's
 * minus the provenance members. The `typeof` guard makes the undefined
 * case yield empty rather than throw.
 */
export function provenanceMetadata(): string {
  const version =
    typeof AGGREGA_PLUGIN_VERSION !== "undefined" ? AGGREGA_PLUGIN_VERSION : "";
  const commit =
    typeof AGGREGA_PLUGIN_COMMIT !== "undefined" ? AGGREGA_PLUGIN_COMMIT : "";

  let meta = "";
  if (version) {
    meta += `,"version":${JSON.stringify(version)}`;
  }
  if (commit) {
    meta += `,"commit":${JSON.stringify(commit)}`;
  }
  return meta;
}
