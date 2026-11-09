// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Fixture governance plugin for the label-transform role, for the Aggrega Wasm runtime.
 *
 * Applies one trivial transform — appends the label
 * `fixture.aggrega-pdk/touched = mutator-typescript` to every node in
 * the mutation graph — and returns one JSON Patch document per node
 * carrying that addition. The host verifies end-to-end that the
 * canonical CBOR payload decoded, the transform applied per node, and
 * the result encoded to a shape that validates against the
 * wasm-mutator schema.
 *
 * The label-transform role's wire carries canonical CBOR in both directions,
 * read and written through the generated codec on the payload type. The
 * patch documents the output carries are JSON, and stay JSON: the socket
 * carries them verbatim and neither side reads into them.
 *
 * Entry convention follows the Extism JS PDK: parameterless function,
 * read input from and write output to the host buffers, expose via
 * module.exports.
 */

import {
  API_VERSION,
  decodeCBORMutatorInput,
  encodeCBORMutatorOutput,
  encodeDescriptor,
  type MutatorOutput,
  type PluginDescriptor,
} from "@aggrega-ai/aggrega-pdk-typescript";

// One operation of an emitted patch document. The socket carries a patch
// document opaquely, so its shape is this fixture's to state rather than the
// wire's — unlike the payload types around it, which are imported from the
// generated ABI so there is one statement of the wire shape.
interface JsonPatchOp {
  op: string;
  path: string;
  value: string;
}

const FIXTURE_LABEL_KEY = "fixture.aggrega-pdk/touched";
const FIXTURE_LABEL_VALUE = "mutator-typescript";

// The static describe() self-report: a governance plugin whose
// mutate export the host validates before dispatch.
function descriptor(): PluginDescriptor {
  return {
    abi_version: API_VERSION,
    plugin_type: "governance",
    description: "Append a fixed label to every node in the mutation graph.",
    exports: [{ name: "mutate" }],
    metadata: { author: "aggrega", license: "Apache-2.0" },
  };
}

// Escapes a JSON Pointer reference token per RFC 6901: "~" becomes "~0"
// and "/" becomes "~1". The order matters — "~" is escaped first so a
// "/" rewritten to "~1" is not re-escaped. The label key carries a "/",
// so escaping is required for the emitted patch path to address a single
// label entry rather than a non-existent nested path.
function escapePointerToken(tok: string): string {
  return tok.replace(/~/g, "~0").replace(/\//g, "~1");
}

// The host calls describe() once at load to read the plugin's static
// self-report before dispatching mutate.
function describe(): void {
  Host.outputString(encodeDescriptor(descriptor()));
}

function mutate(): void {
  const input = decodeCBORMutatorInput(new Uint8Array(Host.inputBytes()));

  // Attachment inputs arrive alongside the graph. settings is a free-form
  // object the host passes through verbatim; the optional "label_value" key
  // overrides the label the fixture writes, defaulting to the language constant
  // when no settings are attached. sources carry the declared definition files
  // as { path, content } — a real mutator parses them to drive its labelling;
  // the fixture simply reads them to demonstrate access.
  let labelValue = FIXTURE_LABEL_VALUE;
  const settings = input.settings as { label_value?: string } | undefined;
  if (settings && typeof settings.label_value === "string" && settings.label_value) {
    labelValue = settings.label_value;
  }
  for (const src of input.sources ?? []) {
    // A real plugin parses src.content (addressed by src.path) here.
    void src;
  }

  // The nodes arrive in a Map, in the order the host sent them. Sort the
  // node IDs so the patch list does not depend on that order; sorting
  // matches the Aggrega determinism discipline and the Go/Rust fixture
  // behaviour. Object.keys() of a Map is empty, so the IDs are read
  // through the Map's own keys().
  const nodeIds = Array.from(input.graph.nodes.keys()).sort();

  const patches: JsonPatchOp[][] = nodeIds.map((nodeId) => [
    {
      op: "add",
      path: `/nodes/${escapePointerToken(nodeId)}/labels/${escapePointerToken(
        FIXTURE_LABEL_KEY,
      )}`,
      value: labelValue,
    },
  ]);

  const out: MutatorOutput = { patches };
  Host.outputBytes(encodeCBORMutatorOutput(out).buffer as ArrayBuffer);
}

module.exports = { mutate, describe };
