// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Fixture policy plugin for the Aggrega Wasm runtime.
//!
//! Emits a deterministic two-verdict [`PolicyOutput`]: one
//! halt verdict and one info verdict. The fixture does not inspect the
//! input graph; the outputs are constants so end-to-end tests can
//! assert exact equality of the emitted shape.
//!
//! The role's payload and result travel as canonical CBOR, so the export takes
//! and returns bytes and writes its result with `PolicyOutput::encode_cbor`.

use aggrega_pdk_rust::{
    encode_descriptor, Diagnostic, PluginDescriptor, PluginDescriptorMeta, PluginExport,
    PolicyOutput, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};

/// The static `describe()` self-report: a v1alpha1 `policy`
/// plugin evaluating the ir stage, whose `verdict_ir` export the host
/// validates before dispatch.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "governance".to_string(),
        description: "Report deterministic halt and info policy verdicts over the build graph."
            .to_string(),
        exports: vec![PluginExport {
            name: "verdict_ir".to_string(),
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
/// self-report before dispatching `verdict_ir`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(encode_descriptor(&descriptor()))
}

#[plugin_fn]
pub fn verdict_ir(_input: Vec<u8>) -> FnResult<Vec<u8>> {
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
    Ok(out.encode_cbor()?)
}
