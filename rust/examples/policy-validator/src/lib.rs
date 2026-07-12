// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Fixture policy-validator plugin for the Aggrega Wasm runtime.
//!
//! Emits a deterministic two-verdict [`PolicyValidatorOutput`]: one
//! deny verdict and one audit verdict. The fixture does not inspect the
//! input graph; the outputs are constants so end-to-end tests can
//! assert exact equality of the emitted shape.

use aggrega_pdk_rust::{PolicyResult, PolicyValidatorOutput};
use extism_pdk::{plugin_fn, FnResult};

/// The static `describe()` self-report: a v1alpha1 `policy-validator`
/// plugin evaluating the ir stage, whose `validate` export the host
/// validates before dispatch.
const PLUGIN_DESCRIPTOR: &str = r#"{"abi_version":"v1alpha1","plugin_type":"policy-validator","stage":"ir","description":"Report deterministic deny and audit policy verdicts over the build graph.","exports":[{"name":"validate"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}"#;

/// The host calls `describe()` once at load to read the plugin's static
/// self-report before dispatching `validate`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(PLUGIN_DESCRIPTOR.to_string())
}

#[plugin_fn]
pub fn validate(_input: String) -> FnResult<String> {
    let out = PolicyValidatorOutput {
        results: vec![
            PolicyResult {
                severity: "deny".to_string(),
                message: "fixture rule B: deliberate fail for end-to-end coverage".to_string(),
            },
            PolicyResult {
                severity: "audit".to_string(),
                message: "fixture rule A: audit note".to_string(),
            },
        ],
    };
    Ok(serde_json::to_string(&out)?)
}
