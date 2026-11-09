// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Reference governance plugin for the `ir-verdict` role, backed by an embedded Rego evaluator.
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
//!   - `deny`  — each message stops the build.
//!   - `warn`  — each message flags the build but lets it proceed.
//!   - `audit` — each message is recorded without affecting the outcome.
//!
//! Those three Rego rule names are Rego's own; the adapter maps them onto the
//! socket's three verdict severities — `deny` lands as `halt`, `warn` as `warn`,
//! and `audit` as `info`.
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
//! as `data.<key>`. The payload arrives as canonical CBOR and is decoded with
//! `PolicyInput::decode_cbor`; the engine reads a JSON document, so the decoded
//! payload is written back out as one, under the names the payload declares.
//!
//! Which rule syntax a policy is written in. Rego is written in two rule
//! syntaxes, and this engine is configured to accept both, so a policy set
//! assembled before the current syntax existed evaluates alongside one written
//! today. Each module chooses for itself: a module in the current syntax
//! declares `import rego.v1` below its package declaration, and a module in the
//! older bracketed-head syntax declares nothing.
//!
//! A reference example rather than a production engine, and this is where the
//! difference bites. A module that parses as the current syntax while omitting
//! the import is read here in the older syntax, and the two can reach opposite
//! decisions from the same text — a rule body that never holds contributes no
//! decision under the current syntax and contributes one under the older syntax.
//! This example does not refuse such a module; it evaluates it. A guest an
//! operator attaches to a real build should refuse it instead, by name, telling
//! the author to add `import rego.v1` — the shipped governance guest does
//! exactly that, and the mechanism is the engine itself: a module written in the
//! older syntax is recognised by not parsing as the current one, so nothing
//! about the module's text has to be guessed at.
//!
//! Loud failure. When evaluating a rule fails — most commonly because the
//! policy calls a builtin the embedded engine does not implement — the plugin
//! returns an error rather than a verdict. The host surfaces that as a build
//! fault naming the plugin, distinct from a deny verdict, so "the engine
//! cannot evaluate this policy" never reads as "the policy allows the build".

use aggrega_pdk_rust::{
    SEVERITY_HALT, SEVERITY_INFO, SEVERITY_WARN,
    encode_descriptor, PluginDescriptor, PluginDescriptorMeta, PluginExport, PolicyInput,
    PolicyOutput, Diagnostic, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};
use regorus::Value;

/// The static `describe()` self-report: a `governance` plugin
/// for the `ir-verdict` role, whose `verdict_ir` export the host validates before
/// dispatch.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "governance".to_string(),
        description:
            "Evaluate operator-supplied Rego policies against the build graph with an embedded Rego engine."
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

/// The host calls `verdict_ir(input)` with the policy payload as canonical CBOR
/// and reads back the fired verdicts in the same encoding. An evaluation failure
/// propagates as an error, which the host surfaces as a build fault.
#[plugin_fn]
pub fn verdict_ir(input: Vec<u8>) -> FnResult<Vec<u8>> {
    let out = evaluate(&input)?;
    Ok(out.encode_cbor()?)
}

/// evaluate loads the attached policies, wires the payload into the Rego
/// engine, and collects the fired verdicts. It returns an error for any
/// genuine load or evaluation failure so the caller surfaces a build fault; an
/// undefined decision rule is not a failure and simply contributes no verdict.
fn evaluate(raw: &[u8]) -> anyhow::Result<PolicyOutput> {
    let payload = PolicyInput::decode_cbor(raw)
        .map_err(|err| anyhow::anyhow!("reading the ir-verdict payload: {err}"))?;

    let mut engine = regorus::Engine::new();
    // Accept the older rule syntax as well as the current one. The engine builds
    // a parser per module, so a module declaring `import rego.v1` is still read
    // in the current syntax whatever the engine-wide default is.
    engine.set_rego_v0(true);

    // Each attached source is a Rego module; content is the raw .rego text.
    for src in &payload.sources {
        engine.add_policy(src.path.clone(), src.content.clone())?;
    }

    // The attachment's settings object becomes the Rego data document.
    if let Some(settings) = payload.settings.as_deref() {
        engine.add_data(Value::from_json_str(settings.get())?)?;
    }

    // The whole payload is the Rego input document.
    engine.set_input(Value::from_json_str(&serde_json::to_string(&payload)?)?);

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
        collect(&obj, "deny", SEVERITY_HALT, &mut results)?;
        collect(&obj, "warn", SEVERITY_WARN, &mut results)?;
        collect(&obj, "audit", SEVERITY_INFO, &mut results)?;
    }
    Ok(PolicyOutput {
        results,
        ..Default::default()
    })
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
///
/// `key` is the Rego decision-rule name the policy author wrote and `severity`
/// is the verdict severity it lands as. The two are separate arguments because
/// they belong to two vocabularies: the Rego side follows Rego's own naming for
/// a decision rule, and the verdict side reports from the closed set the socket
/// declares. This adapter is where one becomes the other.
fn collect(
    package_obj: &Value,
    key: &str,
    severity: &str,
    out: &mut Vec<Diagnostic>,
) -> anyhow::Result<()> {
    let obj = match package_obj.as_object() {
        Ok(o) => o,
        Err(_) => return Ok(()),
    };
    let value = match obj.get(&Value::from(key)) {
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
    anyhow::bail!("policy key {key} must be a set or array of message strings")
}

/// verdict builds one Diagnostic from a message value, requiring the message
/// to be a string so a malformed collection surfaces loudly rather than as a
/// dropped or stringified verdict.
fn verdict(severity: &str, message: &Value) -> anyhow::Result<Diagnostic> {
    Ok(Diagnostic {
        severity: severity.to_string(),
        message: message.as_string()?.to_string(),
        ..Default::default()
    })
}
