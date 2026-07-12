// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Fixture policy-validator plugin for the Aggrega Wasm runtime.
 *
 * Emits a deterministic two-verdict PolicyValidatorOutput: one deny
 * verdict and one audit verdict. The fixture does not inspect input
 * (Host.inputString() is read for parity with the runtime's expected
 * call shape but the value is unused); the outputs are constants so
 * end-to-end tests can assert exact shape across all three languages.
 */

interface PolicyResult {
  severity: string;
  message: string;
}

interface PolicyValidatorOutput {
  results: PolicyResult[];
}

// The static describe() self-report: a v1alpha1 policy-validator plugin
// evaluating the ir stage, whose validate export the host validates before
// dispatch.
const PLUGIN_DESCRIPTOR =
  '{"abi_version":"v1alpha1","plugin_type":"policy-validator","stage":"ir","description":"Report deterministic deny and audit policy verdicts over the build graph.","exports":[{"name":"validate"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}';

// The host calls describe() once at load to read the plugin's static
// self-report before dispatching validate.
function describe(): void {
  Host.outputString(PLUGIN_DESCRIPTOR);
}

function validate(): void {
  // Read input for runtime parity with the other fixtures' shape;
  // the policy-validator fixture's contract is constant output, so
  // the input value is discarded.
  Host.inputString();

  const out: PolicyValidatorOutput = {
    results: [
      { severity: "deny", message: "fixture rule B: deliberate fail for end-to-end coverage" },
      { severity: "audit", message: "fixture rule A: audit note" },
    ],
  };
  Host.outputString(JSON.stringify(out));
}

module.exports = { validate, describe };
