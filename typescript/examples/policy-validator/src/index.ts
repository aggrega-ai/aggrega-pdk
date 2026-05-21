// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Fixture policy-validator plugin for the Aggrega Wasm runtime.
 *
 * Emits a deterministic two-result PolicyValidatorOutput: one pass
 * result and one fail result. The fixture does not inspect input
 * (Host.inputString() is read for parity with the runtime's expected
 * call shape but the value is unused); the outputs are constants so
 * end-to-end tests can assert exact shape across all three languages.
 */

interface PolicyResult {
  pass: boolean;
  message: string;
}

interface PolicyValidatorOutput {
  results: PolicyResult[];
}

function validate(): void {
  // Read input for runtime parity with the other fixtures' shape;
  // the policy-validator fixture's contract is constant output, so
  // the input value is discarded.
  Host.inputString();

  const out: PolicyValidatorOutput = {
    results: [
      { pass: true, message: "fixture rule A: ok" },
      { pass: false, message: "fixture rule B: deliberate fail for end-to-end coverage" },
    ],
  };
  Host.outputString(JSON.stringify(out));
}

module.exports = { validate };
