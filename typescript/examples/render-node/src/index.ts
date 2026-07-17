// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Fixture provider render plugin for the Aggrega Wasm runtime.
 *
 * Applies one trivial, deterministic transform — emits a single output
 * file at a fixed path whose bytes are `rendered-node:` followed by the
 * node id — and returns it as the sole RenderedFile of a RenderNodeOutput.
 * The transform is identical across the per-language render fixtures, so
 * the same RenderNodeInput yields equivalent output whichever language
 * rendered it. The host verifies end-to-end that input JSON deserialised,
 * the render transform ran against the node projection, and output JSON
 * serialised to a shape whose file content round-trips through the render
 * socket's base64 wire encoding.
 *
 * Entry convention follows the Extism JS PDK: parameterless function,
 * read input via Host.inputString(), write output via
 * Host.outputString(), expose via module.exports.
 */

import {
  API_VERSION,
  encodeDescriptor,
  type PluginDescriptor,
} from "@aggrega-ai/aggrega-pdk-typescript";

interface IRNodeProjection {
  id: string;
}

interface RenderNodeInput {
  node: IRNodeProjection;
}

interface RenderedFile {
  path: string;
  content: string;
}

interface RenderNodeOutput {
  files: RenderedFile[];
}

// The fixed on-disk path the fixture emits its single file at. Constant
// across the per-language render fixtures so their outputs compare equal.
const OUTPUT_PATH = "parity.txt";

// The fixed prefix prepended to the node id to form the emitted file's
// bytes. Constant across the per-language render fixtures so identical
// input produces identical content.
const CONTENT_PREFIX = "rendered-node:";

// The static describe() self-report: a v1alpha1 provider plugin whose
// render_node export the host validates before dispatch.
function descriptor(): PluginDescriptor {
  return {
    abi_version: API_VERSION,
    plugin_type: "provider",
    description:
      "Render one IR node to a single output file whose bytes are the node id under a fixed prefix.",
    capabilities: ["parity"],
    exports: [
      { name: "render_node", input_pattern: "b", stability: "experimental" },
    ],
    metadata: { author: "aggrega", license: "Apache-2.0" },
  };
}

// Standard base64 alphabet (RFC 4648) with padding, matching Go's
// base64.StdEncoding — the render socket carries file content as base64
// on the JSON wire, so the guest encodes the raw bytes and the host
// decodes them verbatim.
const BASE64_ALPHABET =
  "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

// base64Encode encodes a byte array to a standard-alphabet, padded base64
// string. Implemented here rather than relying on a runtime btoa/Buffer,
// which the QuickJS-based guest runtime does not guarantee.
function base64Encode(bytes: number[]): string {
  let out = "";
  for (let i = 0; i < bytes.length; i += 3) {
    const b0 = bytes[i];
    const b1 = i + 1 < bytes.length ? bytes[i + 1] : 0;
    const b2 = i + 2 < bytes.length ? bytes[i + 2] : 0;
    out += BASE64_ALPHABET[b0 >> 2];
    out += BASE64_ALPHABET[((b0 & 0x03) << 4) | (b1 >> 4)];
    out += i + 1 < bytes.length ? BASE64_ALPHABET[((b1 & 0x0f) << 2) | (b2 >> 6)] : "=";
    out += i + 2 < bytes.length ? BASE64_ALPHABET[b2 & 0x3f] : "=";
  }
  return out;
}

// utf8Bytes returns the UTF-8 byte values of s. The fixture's inputs are
// ASCII (the node id and the fixed prefix), so a code-unit walk yields the
// same bytes the Rust fixture's UTF-8 encoding produces; a guard rejects a
// non-ASCII code unit rather than emit bytes that would diverge from the
// other language's output.
function utf8Bytes(s: string): number[] {
  const bytes: number[] = [];
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    if (c > 0x7f) {
      throw new Error("render-node fixture expects ASCII input");
    }
    bytes.push(c);
  }
  return bytes;
}

// The host calls describe() once at load to read the plugin's static
// self-report before dispatching render_node.
function describe(): void {
  Host.outputString(encodeDescriptor(descriptor()));
}

function render_node(): void {
  const input: RenderNodeInput = JSON.parse(Host.inputString());
  const content = CONTENT_PREFIX + input.node.id;
  const out: RenderNodeOutput = {
    files: [{ path: OUTPUT_PATH, content: base64Encode(utf8Bytes(content)) }],
  };
  Host.outputString(JSON.stringify(out));
}

module.exports = { render_node, describe };
