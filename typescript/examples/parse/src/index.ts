// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

/**
 * Fixture parser plugin for the Aggrega Wasm runtime.
 *
 * A parser turns a foreign source text into a canonical structured tree, so
 * any consumer can read the tree without carrying its own language parser.
 * This example is a minimal scaffold rather than a real language parser: it
 * self-describes as a parser plugin advertising the parse:demo format, and its
 * parse export echoes the input it receives back as a small structured tree.
 * The point is to exercise both directions of the generated parse codec — the
 * export decodes a ParseInput and encodes a ParseOutput — not to implement a
 * grammar.
 *
 * The parser socket carries canonical CBOR in both directions, read and
 * written through the generated codec on the payload type: the input with
 * decodeCBORParseInput and the output with encodeCBORParseOutput. The tree the
 * output carries is a JSON document, and stays one: it crosses as a byte string
 * holding that document verbatim.
 *
 * Entry convention follows the Extism JS PDK: parameterless function, read
 * input from and write output to the host buffers, expose via module.exports.
 * The parse codec is consumed from the TypeScript Plugin Development Kit, which
 * re-exports it from the portable ABI package.
 */

import {
  API_VERSION,
  decodeCBORParseInput,
  encodeCBORParseOutput,
  encodeDescriptor,
  type PluginDescriptor,
} from "@aggrega-ai/aggrega-pdk-typescript";

// The static describe() self-report: a parser plugin whose parse
// export handles the parse:demo format. The host reads this once at load and
// validates it before invoking parse.
function descriptor(): PluginDescriptor {
  return {
    abi_version: API_VERSION,
    plugin_type: "parser",
    description:
      "Echo a source text back as a small structured tree — a minimal parser scaffold.",
    capabilities: ["parse:demo"],
    exports: [
      {
        name: "parse",
      },
    ],
    metadata: { author: "aggrega", license: "Apache-2.0" },
  };
}

// The host calls describe() once at load to read the plugin's static
// self-report before dispatching parse.
function describe(): void {
  Host.outputString(encodeDescriptor(descriptor()));
}

// parse decodes a ParseInput and returns a ParseOutput. A real parser would
// read the bytes as the named format and build the canonical tree; this
// scaffold echoes the input as a small object tree so the example exercises
// both codec directions without a grammar.
function parse(): void {
  const input = decodeCBORParseInput(new Uint8Array(Host.inputBytes()));
  const tree = { format: input.format, bytes: input.bytes };
  Host.outputBytes(encodeCBORParseOutput({ tree }).buffer as ArrayBuffer);
}

module.exports = { parse, describe };
