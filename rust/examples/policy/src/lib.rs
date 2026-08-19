// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Fixture policy plugin for the Aggrega Wasm runtime.
//!
//! Emits a deterministic two-verdict [`PolicyOutput`]: one
//! halt verdict and one info verdict. The fixture does not inspect the
//! input graph; the outputs are constants so end-to-end tests can
//! assert exact equality of the emitted shape.

use aggrega_pdk_rust::{
    encode_descriptor, Diagnostic, PluginDescriptor, PluginDescriptorMeta, PluginExport,
    PolicyOutput, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};

/// The static `describe()` self-report: a v1alpha1 `policy`
/// plugin evaluating the ir stage, whose `validate` export the host
/// validates before dispatch.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "governance".to_string(),
        description: "Report deterministic halt and info policy verdicts over the build graph."
            .to_string(),
        exports: vec![PluginExport {
            name: "validate".to_string(),
            ..Default::default()
        }],
        metadata: Some(Box::new(PluginDescriptorMeta {
            author: "aggrega".to_string(),
            license: "Apache-2.0".to_string(),
            ..Default::default()
        })),
        ..Default::default()
    }
}

/// The host calls `describe()` once at load to read the plugin's static
/// self-report before dispatching `validate`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(encode_descriptor(&descriptor()))
}

#[plugin_fn]
pub fn validate(_input: String) -> FnResult<String> {
    let out = PolicyOutput {
        results: vec![
            Diagnostic {
                severity: "halt".to_string(),
                message: "fixture rule B: deliberate fail for end-to-end coverage".to_string(),
                ..Default::default()
            },
            Diagnostic {
                severity: "info".to_string(),
                message: "fixture rule A: advisory note".to_string(),
                ..Default::default()
            },
        ],
        ..Default::default()
    };
    Ok(serde_json::to_string(&out)?)
}
