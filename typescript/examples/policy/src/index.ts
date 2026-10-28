// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Fixture governance plugin for the ir-verdict role, for the Aggrega Wasm runtime.
 *
 * Emits a deterministic two-verdict PolicyOutput: one halt
 * verdict and one info verdict. The fixture does not inspect input
 * (Host.inputBytes() is read for parity with the runtime's expected
 * call shape but the value is unused); the outputs are constants so
 * end-to-end tests can assert exact shape across all three languages.
 * The role's result travels as canonical CBOR, written with
 * encodeCBORPolicyOutput.
 */

import {
  API_VERSION,
  encodeCBORPolicyOutput,
  encodeDescriptor,
  type Diagnostic,
  type PluginDescriptor,
  type PolicyOutput,
} from "@aggrega-ai/aggrega-pdk-typescript";

// The static describe() self-report: a v1alpha1 governance plugin
// for the ir-verdict role, whose verdict_ir export the host validates before
// dispatch.
function descriptor(): PluginDescriptor {
  return {
    abi_version: API_VERSION,
    plugin_type: "governance",
    description:
      "Report deterministic halt and info policy verdicts over the build graph.",
    exports: [{ name: "verdict_ir" }],
    metadata: { author: "aggrega", license: "Apache-2.0" },
  };
}

// The host calls describe() once at load to read the plugin's static
// self-report before dispatching verdict_ir.
function describe(): void {
  Host.outputString(encodeDescriptor(descriptor()));
}

function verdict_ir(): void {
  // Read input for runtime parity with the other fixtures' shape;
  // the policy fixture's contract is constant output, so
  // the input value is discarded.
  Host.inputBytes();

  // Each verdict is one shared diagnostic. severity and message are the
  // required pair; code, rule_id, source_ref, location and suggest are
  // available on the same entry and stay unset because this fixture
  // offers none of them.
  const results: Diagnostic[] = [
    { severity: "halt", message: "fixture rule B: deliberate fail for end-to-end coverage" },
    { severity: "info", message: "fixture rule A: advisory note" },
  ];
  const out: PolicyOutput = { results };
  Host.outputBytes(encodeCBORPolicyOutput(out).buffer as ArrayBuffer);
}

module.exports = { verdict_ir, describe };
