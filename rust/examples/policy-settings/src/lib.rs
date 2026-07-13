// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Typed-settings policy-validator fixture plugin for the Aggrega Wasm runtime.
//!
//! This example demonstrates a plugin that types its attachment settings. Its
//! `describe()` self-report carries a `settings_schema` — a JSON Schema the host
//! compiles at load and validates the operator's `with.settings` against before
//! dispatching `validate`. The schema requires an integer `max_replicas` of at
//! least 1 and forbids unknown keys, so an attachment whose settings omit
//! `max_replicas`, give it the wrong type, or add an unknown key halts the build
//! loudly (naming the offending key) before this plugin ever runs.
//!
//! Because conformance is enforced by the host ahead of dispatch, `validate`
//! reads the typed setting directly and emits a single audit verdict echoing the
//! value it read — proof that a conforming setting flowed through to the guest as
//! the declared type.

use aggrega_pdk_rust::{PolicyResult, PolicyValidatorInput, PolicyValidatorOutput};
use extism_pdk::{plugin_fn, FnResult, WithReturnCode};
use serde::Deserialize;

/// The static `describe()` self-report: a v1alpha1 `policy-validator` plugin
/// evaluating the ir stage whose settings are typed by an inline JSON Schema.
/// The host compiles the `settings_schema` at load and validates the
/// attachment's `with.settings` against it before dispatching `validate`.
const PLUGIN_DESCRIPTOR: &str = r#"{"abi_version":"v1alpha1","plugin_type":"policy-validator","stage":"ir","description":"Enforce a typed replica ceiling declared through the attachment settings schema.","exports":[{"name":"validate"}],"settings_schema":{"type":"object","properties":{"max_replicas":{"type":"integer","minimum":1}},"required":["max_replicas"],"additionalProperties":false},"metadata":{"author":"aggrega","license":"Apache-2.0"}}"#;

/// The typed shape of this plugin's attachment settings. The host guarantees the
/// operator's `with.settings` conforms to the declared `settings_schema` before
/// `validate` runs, so this deserialization always succeeds for a dispatched call.
#[derive(Deserialize)]
struct Settings {
    max_replicas: i64,
}

/// The host calls `describe()` once at load to read the plugin's static
/// self-report — including its settings schema — before dispatching `validate`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(PLUGIN_DESCRIPTOR.to_string())
}

#[plugin_fn]
pub fn validate(input: String) -> FnResult<String> {
    let parsed: PolicyValidatorInput = serde_json::from_str(&input)?;

    // Settings conformance was enforced by the host against the declared
    // settings_schema before dispatch, so the typed read below is total for any
    // call that reaches this plugin.
    let raw = parsed.settings.ok_or_else(|| {
        WithReturnCode::new(
            extism_pdk::Error::msg("settings absent despite a required settings schema"),
            1,
        )
    })?;
    let settings: Settings = serde_json::from_str(raw.get())?;

    let out = PolicyValidatorOutput {
        results: vec![PolicyResult {
            severity: "audit".to_string(),
            message: format!(
                "typed settings applied: max_replicas={}",
                settings.max_replicas
            ),
        }],
    };
    Ok(serde_json::to_string(&out)?)
}
