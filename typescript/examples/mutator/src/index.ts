// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Fixture mutator plugin for the Aggrega Wasm runtime.
 *
 * Applies one trivial transform — appends the label
 * `fixture.aggrega-pdk/touched = mutator-typescript` to every node in
 * the MutatorGraph — and returns one JSON Patch document per node
 * carrying that addition. The host verifies end-to-end that input
 * JSON deserialised, the transform applied per node, and output JSON
 * serialised to a shape that validates against the wasm-mutator JSON
 * Schema.
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

interface MutatorNode {
  component: string;
  resolved_version: string;
  labels: Record<string, string>;
}

interface MutatorInput {
  compilation_context: {
    release_name: string;
    environment: string;
  };
  graph: {
    nodes: Record<string, MutatorNode>;
  };
}

interface JsonPatchOp {
  op: string;
  path: string;
  value: string;
}

interface MutatorOutput {
  patches: JsonPatchOp[][];
}

const FIXTURE_LABEL_KEY = "fixture.aggrega-pdk/touched";
const FIXTURE_LABEL_VALUE = "mutator-typescript";

// The static describe() self-report: a v1alpha1 mutator plugin whose
// mutate export the host validates before dispatch.
function descriptor(): PluginDescriptor {
  return {
    abi_version: API_VERSION,
    plugin_type: "mutator",
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
  const input: MutatorInput = JSON.parse(Host.inputString());

  // Sort node IDs for deterministic patch ordering — Object.keys()
  // returns insertion order, which is stable for a single input but
  // varies across map operations; sorting matches the Aggrega
  // determinism discipline and the Go/Rust fixture behaviour.
  const nodeIds = Object.keys(input.graph.nodes).sort();

  const patches: JsonPatchOp[][] = nodeIds.map((nodeId) => [
    {
      op: "add",
      path: `/nodes/${escapePointerToken(nodeId)}/labels/${escapePointerToken(
        FIXTURE_LABEL_KEY,
      )}`,
      value: FIXTURE_LABEL_VALUE,
    },
  ]);

  const out: MutatorOutput = { patches };
  Host.outputString(JSON.stringify(out));
}

module.exports = { mutate, describe };
