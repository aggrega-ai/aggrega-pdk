// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Fixture provider render plugin for the Aggrega Wasm runtime.
 *
 * Applies two trivial, deterministic transforms and returns each as a
 * RenderedFile of a RenderNodeOutput: one file whose bytes are
 * `rendered-node:` followed by the node id, and one reporting the node's
 * attachments as `alias=sensitive|plain` in the order this guest observes
 * them. The first transform is identical across the per-language render
 * fixtures, so the same RenderNodeInput yields the same node-id echo
 * whichever language rendered it. The second is deliberately allowed to
 * differ: it reports what this language's own map container does with an
 * author-ordered map, which is a property of the language rather than of
 * the socket. The host verifies end-to-end that input JSON deserialised,
 * the render transform ran against the node projection, an attachment's
 * sensitivity marking is readable from the payload alone, and output JSON
 * serialised to a shape whose file content round-trips through the render
 * socket's base64 wire encoding. Beside the files it reports two render facts
 * about what it emitted — the paths it wrote and the node it echoed — in a fixed
 * order that is deliberately not name order, and the host verifies they arrive
 * in that order, each value byte for byte as written here.
 *
 * Entry convention follows the Extism JS PDK: parameterless function,
 * read input via Host.inputString(), write output via
 * Host.outputString(), expose via module.exports.
 */

import {
  API_VERSION,
  encodeDescriptor,
  type PluginDescriptor,
  type RenderFact,
} from "@aggrega-ai/aggrega-pdk-typescript";

// A deliberately narrow view of the node projection: the fixture reads only
// what it echoes. The attachment entry is typed to the one field this fixture
// reports — the `sensitive` field the IR feature-entry shape carries — rather
// than to the whole entry, so the fixture stays legible while still reading the
// marking from the payload instead of guessing it from the alias.
//
// What that field means is worth stating precisely, because a guest that reads
// it as more than this marks values that are not secret:
//
//   - It describes the ATTACHMENT, never one datum. It is true when the
//     attachment handles material declared sensitive anywhere, and says nothing
//     about the specific value any given variable in the render context carries.
//     A guest that emitted a target-native sensitivity primitive per rendered
//     value on the strength of this field would mark values that are not secret.
//   - The host derives it from either of two independent declarations: the
//     attached Feature declaring a sensitive leaf in its values, infra or
//     provides pillar, or the host Component's attachment slot of the same name
//     declaring that whatever binds there handles secret material. Those are
//     different claims — a leaf says this datum is secret material, a slot says
//     whatever binds here handles secret material — and either alone is enough.
//   - The wire field carries that derived answer; it does not declare it. The
//     declarations live in the catalog entities the host read, so a guest cannot
//     tell from this field alone which of the two fired, or which leaf carried
//     the marking.
//
// Optional because the host omits the field from the payload when the
// derivation produced false; an absent field is read as unmarked below.
interface IRFeatureEntryView {
  sensitive?: boolean;
}

interface IRNodeProjection {
  id: string;
  features?: Record<string, IRFeatureEntryView>;
}

interface RenderNodeInput {
  node: IRNodeProjection;
}

interface RenderedFile {
  path: string;
  content: string;
  format?: string;
}

interface RenderNodeOutput {
  files: RenderedFile[];
  render_facts?: RenderFact[];
}

// The fixed on-disk path the fixture emits its node-id echo at. Constant
// across the per-language render fixtures so their outputs compare equal.
const OUTPUT_PATH = "parity.txt";

// The fixed on-disk path the fixture emits its attachment echo at. Constant
// across the per-language render fixtures; the CONTENT is deliberately allowed
// to differ per language, because it reports what this language's own map
// container did with an author-ordered attachment map.
const ATTACHMENTS_PATH = "attachments.txt";

// The fixed prefix prepended to the node id to form the emitted file's
// bytes. Constant across the per-language render fixtures so identical
// input produces identical content.
const CONTENT_PREFIX = "rendered-node:";

// The format the fixture declares its emitted file was rendered in, so the
// host can route the file to a matching parser. A provider that declares no
// format leaves the field unset and the host infers it from the extension.
const OUTPUT_FORMAT = "text";

// The static describe() self-report: a v1alpha1 provider plugin whose
// render_node export the host validates before dispatch.
function descriptor(): PluginDescriptor {
  return {
    abi_version: API_VERSION,
    plugin_type: "provider",
    description:
      "Render one IR node to a node-id echo file and an attachment-report file.",
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

// attachmentEcho reports the node's attachments as `alias=sensitive|plain`,
// comma-joined, in the order this guest observes them.
function attachmentEcho(input: RenderNodeInput): string {
  const features = input.node.features ?? {};
  return Object.keys(features)
    .map((alias) => `${alias}=${features[alias].sensitive ? "sensitive" : "plain"}`)
    .join(",");
}

// renderFacts returns the facts this fixture reports about what it emitted,
// identical across the per-language render fixtures so the host can compare them
// byte for byte. A fact is a name and a list of entries, and both are this
// provider's own vocabulary: the host carries them in the order listed, refuses a
// value that is not a list, and learns nothing else about either. Every value is
// a list even with one entry, because the host joins the lists one name gets from
// every render of a node. JSON.stringify writes an object's keys in insertion
// order, so `prefix` deliberately precedes `id`.
function renderFacts(nodeId: string): RenderFact[] {
  return [
    { name: "emitted_paths", value: [OUTPUT_PATH, ATTACHMENTS_PATH] },
    { name: "echoed_node", value: [{ prefix: CONTENT_PREFIX, id: nodeId }] },
  ];
}

function render_node(): void {
  const input: RenderNodeInput = JSON.parse(Host.inputString());
  const content = CONTENT_PREFIX + input.node.id;
  const out: RenderNodeOutput = {
    files: [
      {
        path: OUTPUT_PATH,
        content: base64Encode(utf8Bytes(content)),
        format: OUTPUT_FORMAT,
      },
      {
        path: ATTACHMENTS_PATH,
        content: base64Encode(utf8Bytes(attachmentEcho(input))),
        format: OUTPUT_FORMAT,
      },
    ],
    render_facts: renderFacts(input.node.id),
  };
  Host.outputString(JSON.stringify(out));
}

module.exports = { render_node, describe };
