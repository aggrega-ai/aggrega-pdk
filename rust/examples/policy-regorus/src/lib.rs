// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Reference policy plugin backed by an embedded Rego evaluator.
//!
//! The plugin evaluates operator-supplied Rego policies against the build's
//! IR graph entirely in-guest: the Rego engine is compiled into the plugin,
//! so the host runs no policy engine of its own and stays free of any
//! policy-language logic. The host reads the attachment's source files and
//! settings at compile time and hands them here; this plugin loads them,
//! evaluates the policy's decision rules, and reports the fired verdicts.
//!
//! Authoring convention. A policy declares its decision rules in a Rego
//! package and exposes up to three collections of message strings:
//!
//!   - `deny`  — each message halts the build.
//!   - `warn`  — each message flags the build but lets it proceed.
//!   - `audit` — each message is recorded without affecting the outcome.
//!
//! Each collection is a set or array of strings; a rule that stays undefined
//! contributes nothing, so a policy that never populates `deny` allows the
//! build. Every package the attached sources declare is evaluated, so a
//! policy may split its rules across packages.
//!
//! Input mapping. The whole policy payload is the Rego `input` document: a
//! policy reaches the IR graph as `input.graph` and the routing paths as
//! `input.precalculated_paths`. The attachment's `settings` object becomes
//! the Rego `data` document, so a policy reads operator-supplied thresholds
//! as `data.<key>`.
//!
//! Loud failure. When evaluating a rule fails — most commonly because the
//! policy calls a builtin the embedded engine does not implement — the plugin
//! returns an error rather than a verdict. The host surfaces that as a build
//! fault naming the plugin, distinct from a deny verdict, so "the engine
//! cannot evaluate this policy" never reads as "the policy allows the build".

use aggrega_pdk_rust::{
    encode_descriptor, PluginDescriptor, PluginDescriptorMeta, PluginExport, PolicyOutput,
    PolicyResult, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};
use regorus::Value;

/// The static `describe()` self-report: a v1alpha1 `policy` plugin
/// evaluating the ir stage, whose `validate` export the host validates before
/// dispatch.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "policy".to_string(),
        stage: "ir".to_string(),
        description:
            "Evaluate operator-supplied Rego policies against the build graph with an embedded Rego engine."
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

/// The host calls `validate(input)` with the policy payload as a JSON string
/// and reads back the fired verdicts. An evaluation failure propagates as an
/// error, which the host surfaces as a build fault.
#[plugin_fn]
pub fn validate(input: String) -> FnResult<String> {
    let out = evaluate(&input)?;
    Ok(serde_json::to_string(&out)?)
}

/// evaluate loads the attached policies, wires the payload into the Rego
/// engine, and collects the fired verdicts. It returns an error for any
/// genuine load or evaluation failure so the caller surfaces a build fault; an
/// undefined decision rule is not a failure and simply contributes no verdict.
fn evaluate(raw: &str) -> anyhow::Result<PolicyOutput> {
    let payload: serde_json::Value = serde_json::from_str(raw)?;

    let mut engine = regorus::Engine::new();

    // Each attached source is a Rego module; content is the raw .rego text.
    if let Some(sources) = payload.get("sources").and_then(|s| s.as_array()) {
        for src in sources {
            let path = src
                .get("path")
                .and_then(|p| p.as_str())
                .unwrap_or("policy.rego");
            let content = src.get("content").and_then(|c| c.as_str()).ok_or_else(|| {
                anyhow::anyhow!("policy source {path:?} carries no string content")
            })?;
            engine.add_policy(path.to_string(), content.to_string())?;
        }
    }

    // The attachment's settings object becomes the Rego data document.
    if let Some(settings) = payload.get("settings") {
        if !settings.is_null() {
            engine.add_data(Value::from_json_str(&settings.to_string())?)?;
        }
    }

    // The whole payload is the Rego input document.
    engine.set_input(Value::from_json_str(raw)?);

    let mut results = Vec::new();
    for pkg in engine.get_packages()? {
        // Evaluate the package to its object once, then read its decision keys.
        // Reading keys off the evaluated object avoids probing rule names a
        // policy may not declare — an undeclared rule path is itself an error,
        // indistinguishable from a real failure, whereas an absent key is
        // simply an absent verdict. A genuine evaluation failure — most often
        // an unimplemented builtin — surfaces here and propagates as a build
        // fault rather than a silent allow.
        let obj = eval_package(&mut engine, &pkg)?;
        collect(&obj, "deny", &mut results)?;
        collect(&obj, "warn", &mut results)?;
        collect(&obj, "audit", &mut results)?;
    }
    Ok(PolicyOutput { results })
}

/// eval_package evaluates a package reference to its object value. A reference
/// query yields a single result carrying a single expression; when the package
/// evaluates to no value the result is treated as undefined and contributes no
/// verdict.
fn eval_package(engine: &mut regorus::Engine, package: &str) -> anyhow::Result<Value> {
    let results = engine.eval_query(package.to_string(), false)?;
    Ok(results
        .result
        .first()
        .and_then(|r| r.expressions.first())
        .map(|e| e.value.clone())
        .unwrap_or(Value::Undefined))
}

/// collect reads one decision key off an evaluated package object and appends a
/// verdict of the given severity for each message it holds. An absent or
/// undefined key yields nothing; a key that is neither a set nor an array of
/// strings is a malformed policy and fails loudly.
fn collect(package_obj: &Value, severity: &str, out: &mut Vec<PolicyResult>) -> anyhow::Result<()> {
    let obj = match package_obj.as_object() {
        Ok(o) => o,
        Err(_) => return Ok(()),
    };
    let value = match obj.get(&Value::from(severity)) {
        Some(v) => v,
        None => return Ok(()),
    };
    if value == &Value::Undefined {
        return Ok(());
    }
    // A set (BTreeSet) iterates in a stable sorted order; an array preserves
    // author order. Either yields a deterministic verdict sequence.
    if let Ok(set) = value.as_set() {
        for msg in set {
            out.push(verdict(severity, msg)?);
        }
        return Ok(());
    }
    if let Ok(arr) = value.as_array() {
        for msg in arr {
            out.push(verdict(severity, msg)?);
        }
        return Ok(());
    }
    anyhow::bail!("policy key {severity} must be a set or array of message strings")
}

/// verdict builds one PolicyResult from a message value, requiring the message
/// to be a string so a malformed collection surfaces loudly rather than as a
/// dropped or stringified verdict.
fn verdict(severity: &str, message: &Value) -> anyhow::Result<PolicyResult> {
    Ok(PolicyResult {
        severity: severity.to_string(),
        message: message.as_string()?.to_string(),
    })
}
