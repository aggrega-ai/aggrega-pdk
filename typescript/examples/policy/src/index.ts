// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Fixture policy plugin for the Aggrega Wasm runtime.
 *
 * Emits a deterministic two-verdict PolicyOutput: one deny
 * verdict and one audit verdict. The fixture does not inspect input
 * (Host.inputString() is read for parity with the runtime's expected
 * call shape but the value is unused); the outputs are constants so
 * end-to-end tests can assert exact shape across all three languages.
 */

import {
  API_VERSION,
  encodeDescriptor,
  type Diagnostic,
  type PluginDescriptor,
  type PolicyOutput,
} from "@aggrega-ai/aggrega-pdk-typescript";

// The static describe() self-report: a v1alpha1 policy plugin
// evaluating the ir stage, whose validate export the host validates before
// dispatch.
function descriptor(): PluginDescriptor {
  return {
    abi_version: API_VERSION,
    plugin_type: "governance",
    description:
      "Report deterministic deny and audit policy verdicts over the build graph.",
    exports: [{ name: "validate" }],
    metadata: { author: "aggrega", license: "Apache-2.0" },
  };
}

// The host calls describe() once at load to read the plugin's static
// self-report before dispatching validate.
function describe(): void {
  Host.outputString(encodeDescriptor(descriptor()));
}

function validate(): void {
  // Read input for runtime parity with the other fixtures' shape;
  // the policy fixture's contract is constant output, so
  // the input value is discarded.
  Host.inputString();

  // Each verdict is one shared diagnostic. severity and message are the
  // required pair; code, rule_id, source_ref, location and suggest are
  // available on the same entry and stay unset because this fixture
  // offers none of them.
  const results: Diagnostic[] = [
    { severity: "deny", message: "fixture rule B: deliberate fail for end-to-end coverage" },
    { severity: "audit", message: "fixture rule A: audit note" },
  ];
  const out: PolicyOutput = { results };
  Host.outputString(JSON.stringify(out));
}

module.exports = { validate, describe };
