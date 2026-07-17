// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Typed `describe()` descriptor builder.
 *
 * A guest constructs a typed {@link PluginDescriptor} — plugin type, exports
 * with their host-function and capability fields, capabilities, a provider
 * contract, commands and their flags, metadata — and encodes it to the wire
 * bytes the host reads once at load, rather than hand-assembling the JSON. The
 * codec omits every absent optional field, so a migrated guest emits the same
 * bytes it did as a hand-written JSON literal.
 */

import {
  encodePluginDescriptor,
  type PluginDescriptor,
  type PluginDescriptorMeta,
} from "@aggrega-ai/aggrega-abi";

/**
 * Build-time provenance constants. They do not exist at runtime unless the
 * plugin's esbuild bundle step injects them with `--define`:
 *
 * ```sh
 * npx esbuild src/index.ts --bundle --format=cjs --target=es2022 \
 *   --platform=neutral --outfile=dist/plugin.js \
 *   --define:AGGREGA_PLUGIN_VERSION='"1.2.3"' \
 *   --define:AGGREGA_PLUGIN_COMMIT='"abc1234"'
 * ```
 *
 * They are declared (not defined) here so the type-checker accepts the
 * references; {@link encodeDescriptor} reads them behind a `typeof` guard so a
 * plain, undefined build yields empty rather than a `ReferenceError`.
 */
declare const AGGREGA_PLUGIN_VERSION: string | undefined;
declare const AGGREGA_PLUGIN_COMMIT: string | undefined;

/**
 * Encodes a plugin's `describe()` self-report to the wire bytes the host reads
 * once at load, folding the build-provenance stamp into the descriptor's
 * metadata.
 *
 * When esbuild `--define`s `AGGREGA_PLUGIN_VERSION` / `AGGREGA_PLUGIN_COMMIT`,
 * the corresponding metadata field is populated, allocating the metadata object
 * when the descriptor declared none. A plain build (constants undefined) leaves
 * metadata untouched, so the descriptor's wire form is byte-identical to one
 * built without the provenance fields. The encoding is the ABI codec's canonical
 * form: fields in schema-declaration order, absent optionals omitted.
 */
export function encodeDescriptor(descriptor: PluginDescriptor): string {
  const version =
    typeof AGGREGA_PLUGIN_VERSION !== "undefined" ? AGGREGA_PLUGIN_VERSION : "";
  const commit =
    typeof AGGREGA_PLUGIN_COMMIT !== "undefined" ? AGGREGA_PLUGIN_COMMIT : "";

  if (version || commit) {
    const metadata: PluginDescriptorMeta = { ...(descriptor.metadata ?? {}) };
    if (version) {
      metadata.version = version;
    }
    if (commit) {
      metadata.commit = commit;
    }
    descriptor = { ...descriptor, metadata };
  }
  return encodePluginDescriptor(descriptor);
}
