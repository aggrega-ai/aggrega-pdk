// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Reference render-stage policy-validator plugin backed by an embedded Rego
//! evaluator.
//!
//! This is the render-stage sibling of the IR-stage embedded-Rego policy
//! plugin. Where the IR-stage plugin evaluates operator-supplied Rego against
//! the build's IR graph, this plugin evaluates operator-supplied Rego against
//! the rendered output artifacts — the files the compiler emits (a Docker
//! Compose file, a CloudFormation template, any YAML or JSON render output).
//! The Rego engine is embedded in the guest, so the host runs no policy engine
//! of its own and carries no policy-language or output-format logic.
//!
//! Output-shape independence. The adapter is not tied to Kubernetes or to any
//! one render target. It parses each artifact's content as one or more YAML or
//! JSON documents (YAML is a superset of JSON, so a JSON template parses too)
//! and exposes the parsed documents to the policy alongside the raw content. A
//! policy therefore reasons over the structured render output of any YAML/JSON
//! target — Compose services, CloudFormation resources — with the same Rego it
//! would write against any structured document.
//!
//! Input mapping. The Rego `input` document is the render payload enriched with
//! the parsed documents:
//!
//!   - `input.artifacts[_].path`        — the artifact's output path.
//!   - `input.artifacts[_].content`     — the raw rendered text, verbatim.
//!   - `input.artifacts[_].documents`   — the parsed YAML/JSON documents at that
//!                                        path, in document order.
//!   - `input.artifacts[_].parse_error` — present only when the content did not
//!                                        parse; carries the parser's message.
//!   - `input.sources`, `input.settings` — the attachment's own sources and
//!                                        settings, carried through verbatim.
//!
//! Each attached source is loaded as a Rego module; the attachment's `settings`
//! object becomes the Rego `data` document, so a policy reads an operator
//! tunable as `data.<key>`.
//!
//! Parse failures do not silently allow. When an artifact's content does not
//! parse as YAML/JSON, the adapter neither crashes nor drops the artifact: it
//! keeps the raw `content`, sets `documents` to an empty array, and records the
//! parser's message under `parse_error`. A policy that reasons over structured
//! documents sees no documents for that artifact, but the malformed artifact
//! stays visible — a policy can `deny` on `input.artifacts[_].parse_error` to
//! make a malformed render a hard failure. The adapter never decides an
//! unparseable artifact is compliant on the policy's behalf; surfacing it keeps
//! the decision with the policy.
//!
//! Loud failure. When evaluating a rule fails — most commonly because the
//! policy calls a builtin the embedded engine does not implement — the plugin
//! returns an error rather than a verdict, exactly as the IR-stage plugin does.
//! The host surfaces that as a build fault naming the plugin, distinct from a
//! deny verdict, so "the engine cannot evaluate this policy" never reads as
//! "the policy allows the build".
//!
//! Authoring convention. A policy declares its decision rules in a Rego package
//! and exposes up to three collections of message strings — `deny` (each halts
//! the build), `warn` (each flags the build but lets it proceed), and `audit`
//! (each is recorded without affecting the outcome). Each collection is a set
//! or array of strings; an undefined rule contributes nothing, so a policy that
//! never populates `deny` allows the build. Every package the attached sources
//! declare is evaluated, so a policy may split its rules across packages.
//!
//! Determinism. The host hands the artifacts already sorted by path; the guest
//! walks them in that order and preserves document order within each artifact.
//! Rego set iteration is sorted, so identical input yields a byte-identical
//! verdict sequence.

use aggrega_abi::RenderPolicyValidatorInput;
use aggrega_pdk_rust::{PolicyResult, PolicyValidatorOutput};
use extism_pdk::{plugin_fn, FnResult};
use regorus::Value;
use serde::Deserialize;
use serde_json::json;

/// The static `describe()` self-report: a v1alpha1 `policy-validator` plugin
/// evaluating the render stage, whose `validate` export the host validates
/// before dispatch.
const PLUGIN_DESCRIPTOR: &str = r#"{"abi_version":"v1alpha1","plugin_type":"policy-validator","stage":"render","description":"Evaluate operator-supplied Rego policies against the rendered YAML/JSON output artifacts with an embedded Rego engine.","exports":[{"name":"validate"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}"#;

/// The host calls `describe()` once at load to read the plugin's static
/// self-report before dispatching `validate`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(PLUGIN_DESCRIPTOR.to_string())
}

/// The host calls `validate(input)` with the render-stage payload as a JSON
/// string and reads back the fired verdicts. A malformed payload or an
/// evaluation failure propagates as an error, which the host surfaces as a
/// build fault.
#[plugin_fn]
pub fn validate(input: String) -> FnResult<String> {
    let out = evaluate(&input)?;
    Ok(serde_json::to_string(&out)?)
}

/// evaluate parses the render payload, wires the attached policies and the
/// parsed render output into the Rego engine, and collects the fired verdicts.
/// It returns an error for any genuine load or evaluation failure so the caller
/// surfaces a build fault; an undefined decision rule is not a failure and
/// simply contributes no verdict. A per-artifact parse failure is not an
/// evaluate-level error — it is surfaced to the policy as `parse_error` on the
/// artifact rather than allowed away.
fn evaluate(raw: &str) -> anyhow::Result<PolicyValidatorOutput> {
    let payload: RenderPolicyValidatorInput = serde_json::from_str(raw)?;

    let mut engine = regorus::Engine::new();

    // Each attached source is a Rego module; content is the raw .rego text.
    for src in &payload.sources {
        engine.add_policy(src.path.clone(), src.content.clone())?;
    }

    // The attachment's settings object becomes the Rego data document.
    if let Some(settings) = &payload.settings {
        engine.add_data(Value::from_json_str(settings.get())?)?;
    }

    // The Rego input document is the payload enriched with each artifact's
    // parsed YAML/JSON documents, so a policy reasons over structured render
    // output rather than raw text.
    let input_doc = build_input(&payload);
    engine.set_input(Value::from_json_str(&input_doc.to_string())?);

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
    Ok(PolicyValidatorOutput { results })
}

/// build_input assembles the Rego input document: each artifact carries its
/// path, its raw content, and its parsed documents; the attachment's sources
/// and settings are carried through so the whole payload is visible to a policy.
fn build_input(payload: &RenderPolicyValidatorInput) -> serde_json::Value {
    let artifacts: Vec<serde_json::Value> = payload
        .artifacts
        .iter()
        .map(|artifact| {
            let mut entry = json!({
                "path": artifact.path,
                "content": artifact.content,
            });
            match parse_documents(&artifact.content) {
                Ok(documents) => {
                    entry["documents"] = serde_json::Value::Array(documents);
                }
                Err(err) => {
                    // Keep the raw content and surface the parse failure; do not
                    // drop the artifact and do not treat it as compliant.
                    entry["documents"] = serde_json::Value::Array(Vec::new());
                    entry["parse_error"] = serde_json::Value::String(err.to_string());
                }
            }
            entry
        })
        .collect();

    let sources: Vec<serde_json::Value> = payload
        .sources
        .iter()
        .map(|source| json!({ "path": source.path, "content": source.content }))
        .collect();

    json!({
        "artifacts": artifacts,
        "sources": sources,
        "settings": settings_value(payload),
    })
}

/// parse_documents parses one artifact's content as a stream of YAML or JSON
/// documents (YAML is a superset of JSON, so a JSON template parses too),
/// returning them in document order. A null document — as a bare `---`
/// separator yields — carries nothing to reason over and is skipped. A parse
/// failure propagates so the caller can surface it as `parse_error` rather than
/// silently drop the artifact.
fn parse_documents(content: &str) -> anyhow::Result<Vec<serde_json::Value>> {
    let mut documents = Vec::new();
    for document in serde_yaml::Deserializer::from_str(content) {
        let value = serde_json::Value::deserialize(document)?;
        if value.is_null() {
            continue;
        }
        documents.push(value);
    }
    Ok(documents)
}

/// settings_value returns the attachment's settings as a JSON value for the
/// input document, or null when the attachment declares none.
fn settings_value(payload: &RenderPolicyValidatorInput) -> serde_json::Value {
    match &payload.settings {
        Some(raw) => serde_json::from_str(raw.get()).unwrap_or(serde_json::Value::Null),
        None => serde_json::Value::Null,
    }
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
