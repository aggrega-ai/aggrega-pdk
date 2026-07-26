// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Reference render-stage policy plugin backed by an embedded Rego
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
//! Native-HCL Terraform output. A native Terraform file (a bare `.tf`) is
//! neither YAML nor JSON, so it is projected through an HCL reader into the same
//! canonical object a `.tf.json` file parses to:
//! `input.artifacts[_].documents[_].resource.<type>.<name>.<attr>`. A policy
//! reads a Terraform resource attribute the same way whichever form the
//! compiler emitted — for an RDS instance, say,
//! `input.artifacts[_].documents[_].resource.aws_db_instance[_].allocated_storage`.
//! Every other artifact — `.tf.json`, `.auto.tfvars.json`, and Compose/CloudFormation
//! YAML or JSON — stays on the YAML/JSON path unchanged. A `.tftpl` is Terraform
//! template text rather than standalone HCL, so it is not projected and, like any
//! non-structured content, surfaces `parse_error`.
//!
//! Input mapping. The Rego `input` document is the render payload enriched with
//! the parsed documents:
//!
//!   - `input.artifacts[_].path`        — the artifact's output path.
//!   - `input.artifacts[_].content`     — the raw rendered text, verbatim.
//!   - `input.artifacts[_].documents`   — the parsed documents at that path, in
//!                                        document order: YAML/JSON documents for
//!                                        a YAML/JSON artifact, or the single
//!                                        projected object for a native `.tf`.
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

use aggrega_abi::RenderPolicyInput;
use aggrega_pdk_rust::{
    encode_descriptor, PluginDescriptor, PluginDescriptorMeta, PluginExport, PolicyOutput,
    Diagnostic, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};
use regorus::Value;
use serde::Deserialize;
use serde_json::json;

/// The static `describe()` self-report: a v1alpha1 `policy` plugin
/// evaluating the render stage, whose `validate` export the host validates
/// before dispatch.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "governance".to_string(),
        description:
            "Evaluate operator-supplied Rego policies against the rendered YAML/JSON output artifacts with an embedded Rego engine."
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
fn evaluate(raw: &str) -> anyhow::Result<PolicyOutput> {
    let payload: RenderPolicyInput = serde_json::from_str(raw)?;

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
    Ok(PolicyOutput {
        results,
        ..Default::default()
    })
}

/// build_input assembles the Rego input document: each artifact carries its
/// path, its raw content, and its parsed documents; the attachment's sources
/// and settings are carried through so the whole payload is visible to a policy.
fn build_input(payload: &RenderPolicyInput) -> serde_json::Value {
    let artifacts: Vec<serde_json::Value> = payload
        .artifacts
        .iter()
        .map(|artifact| {
            let mut entry = json!({
                "path": artifact.path,
                "content": artifact.content,
            });
            // A native-HCL Terraform file is neither YAML nor JSON, so it is
            // projected through the HCL reader; every other artifact — including
            // `.tf.json`, `.auto.tfvars.json`, and `.tftpl` template text — stays
            // on the YAML/JSON path.
            let parsed = if is_native_hcl(&artifact.path) {
                project_hcl(&artifact.content)
            } else {
                parse_documents(&artifact.content)
            };
            match parsed {
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

/// is_native_hcl reports whether an artifact path is a native-HCL Terraform file
/// — a bare `.tf`. A `.tf.json` is already JSON and a `.tftpl` is Terraform
/// template text rather than standalone HCL, so both stay on the YAML/JSON path.
/// (`.tf.json` ends in `.json`, so it fails the `.tf` suffix test on its own;
/// the explicit exclusion states the intent and guards the boundary.)
fn is_native_hcl(path: &str) -> bool {
    path.ends_with(".tf") && !path.ends_with(".tf.json")
}

/// project_hcl projects one native-HCL Terraform artifact into its canonical
/// object form. HCL is neither YAML nor JSON, so it is read with an HCL reader
/// that yields the same shape a `.tf.json` file parses to —
/// `{"resource": {"<type>": {"<name>": {<attrs>}}}}` — so a policy reads a
/// resource attribute the same way regardless of which Terraform form the
/// compiler emitted. It returns the single projected document in a one-element
/// vector so it flows through the same `documents` array the YAML/JSON path
/// produces. Object keys sort deterministically. A parse failure propagates so
/// the caller can surface it as `parse_error` rather than silently drop the
/// artifact.
fn project_hcl(content: &str) -> anyhow::Result<Vec<serde_json::Value>> {
    let value: serde_json::Value = hcl::from_str(content)?;
    Ok(vec![value])
}

/// settings_value returns the attachment's settings as a JSON value for the
/// input document, or null when the attachment declares none.
fn settings_value(payload: &RenderPolicyInput) -> serde_json::Value {
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
fn collect(package_obj: &Value, severity: &str, out: &mut Vec<Diagnostic>) -> anyhow::Result<()> {
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

#[cfg(test)]
mod tests {
    use super::*;
    use aggrega_abi::RenderedArtifact;

    // The rendered main.tf of the terraform-rds worked example, captured from a
    // live build of that example. Native HCL, not YAML or JSON — the projection
    // under test turns it into the canonical Terraform object.
    const RENDERED_MAIN_TF: &str = r####"# Aggrega build provenance
# aggrega.ai/aggregate-name: aggregate
# aggrega.ai/boundary-path: infra
# aggrega.ai/compiler-version: (devel)
# aggrega.ai/component-id: db
# aggrega.ai/environment: production
# aggrega.ai/provider: terraform
# aggrega.ai/provider-version: 0.4.0-97-g89c68d8
# aggrega.ai/recipe-uri: mariadb-rds:1.0.0
resource "aws_db_instance" "db" {
  identifier     = "db"
  engine         = "mariadb"
  engine_version = "11.4"
  port           = 3306

  ## ── Compute ──────────────────────────────────────────────
  instance_class = "db.t3.small"

  ## ── Storage ──────────────────────────────────────────────
  allocated_storage = 100
  storage_type      = "gp3"

  ## ── Database ─────────────────────────────────────────────
  db_name  = "appdb"
  username = "admin"

  ## Black-Box: RDS manages the password via Secrets Manager.
  manage_master_user_password = true

  ## ── HA ───────────────────────────────────────────────────
  multi_az = true

  ## ── Backup ───────────────────────────────────────────────
  backup_retention_period = 14

  ## ── Provider-specific overrides ──────────────────────────
}
"####;

    // Builds a one-artifact render payload plus a single Rego source, as the
    // JSON string the export boundary hands to evaluate.
    fn payload_json(artifact_path: &str, content: &str, rego: &str) -> String {
        json!({
            "artifacts": [{ "path": artifact_path, "content": content }],
            "sources": [{ "path": "policy.rego", "content": rego }],
        })
        .to_string()
    }

    fn one_artifact_input(path: &str, content: &str) -> RenderPolicyInput {
        RenderPolicyInput {
            artifacts: vec![RenderedArtifact {
                path: path.to_string(),
                content: content.to_string(),
                ..Default::default()
            }],
            ..Default::default()
        }
    }

    // A native .tf artifact projects into the canonical Terraform object shape
    // resource.<type>.<name>.<attr>, with attribute types preserved.
    #[test]
    fn native_tf_projects_to_terraform_shape() {
        let input = one_artifact_input("infra/terraform/main.tf", RENDERED_MAIN_TF);
        let doc = build_input(&input);
        let artifact = &doc["artifacts"][0];
        assert!(
            artifact.get("parse_error").is_none(),
            "a well-formed .tf must not carry parse_error: {artifact}"
        );
        let db = &artifact["documents"][0]["resource"]["aws_db_instance"]["db"];
        assert_eq!(db["allocated_storage"], json!(100));
        assert_eq!(db["port"], json!(3306));
        assert_eq!(db["manage_master_user_password"], json!(true));
        assert_eq!(db["engine"], json!("mariadb"));
        // Exactly one projected document for the native .tf artifact.
        assert_eq!(artifact["documents"].as_array().unwrap().len(), 1);
    }

    // A policy reads a Terraform resource attribute out of the projected object
    // and fires a deny — the load-bearing end-to-end proof.
    #[test]
    fn deny_fires_on_projected_hcl_attribute() {
        let rego = r#"
package aggrega.render.tf

import rego.v1

deny contains msg if {
    some artifact in input.artifacts
    some document in artifact.documents
    some name
    storage := document.resource.aws_db_instance[name].allocated_storage
    storage > 50
    msg := sprintf("aws_db_instance %v allocated_storage %v exceeds the 50Gi ceiling", [name, storage])
}
"#;
        let out = evaluate(&payload_json("infra/terraform/main.tf", RENDERED_MAIN_TF, rego))
            .expect("evaluate must succeed");
        assert_eq!(out.results.len(), 1, "exactly one deny should fire: {:?}", out.results);
        let v = &out.results[0];
        assert_eq!(v.severity, "deny");
        // The resource is addressed at resource.aws_db_instance.db, so the bound
        // name is "db" and the read attribute is 100.
        assert!(v.message.contains("db"), "message names the resource: {}", v.message);
        assert!(v.message.contains("100"), "message carries the read value: {}", v.message);
    }

    // A deny keyed on port == 3306 also fires, confirming the projected scalar
    // is a number the policy compares directly.
    #[test]
    fn deny_fires_on_projected_port() {
        let rego = r#"
package aggrega.render.tf.port

import rego.v1

deny contains msg if {
    some document in input.artifacts[_].documents
    document.resource.aws_db_instance[_].port == 3306
    msg := "database exposes the default MariaDB port 3306"
}
"#;
        let out = evaluate(&payload_json("infra/terraform/main.tf", RENDERED_MAIN_TF, rego))
            .expect("evaluate must succeed");
        assert_eq!(out.results.len(), 1, "port deny should fire: {:?}", out.results);
        assert_eq!(out.results[0].message, "database exposes the default MariaDB port 3306");
    }

    // A malformed .tf does not silently allow: documents stays empty, parse_error
    // is set, and a policy denying on parse_error fires.
    #[test]
    fn malformed_tf_preserves_parse_error() {
        // Unterminated block — not valid HCL.
        let bad = "resource \"aws_db_instance\" \"db\" {\n  allocated_storage = 100\n";
        let input = one_artifact_input("infra/terraform/main.tf", bad);
        let doc = build_input(&input);
        let artifact = &doc["artifacts"][0];
        assert_eq!(
            artifact["documents"].as_array().unwrap().len(),
            0,
            "malformed .tf yields no documents"
        );
        assert!(
            artifact["parse_error"].is_string(),
            "malformed .tf sets parse_error: {artifact}"
        );

        let rego = r#"
package aggrega.render.tf.malformed

import rego.v1

deny contains msg if {
    some artifact in input.artifacts
    artifact.parse_error
    msg := "render artifact failed to parse"
}
"#;
        let out = evaluate(&payload_json("infra/terraform/main.tf", bad, rego))
            .expect("evaluate must succeed");
        assert_eq!(out.results.len(), 1, "parse_error deny should fire: {:?}", out.results);
        assert_eq!(out.results[0].message, "render artifact failed to parse");
    }

    // A .tf.json is already JSON: it stays on the YAML/JSON path and parses into
    // the same {resource:...} object, confirming the HCL branch excludes it.
    #[test]
    fn tf_json_stays_on_yaml_json_path() {
        let json_tf = r#"{"resource":{"aws_db_instance":{"db":{"allocated_storage":100,"port":3306}}}}"#;
        let input = one_artifact_input("infra/terraform/main.tf.json", json_tf);
        let doc = build_input(&input);
        let artifact = &doc["artifacts"][0];
        assert!(
            artifact.get("parse_error").is_none(),
            "valid .tf.json must not carry parse_error: {artifact}"
        );
        let db = &artifact["documents"][0]["resource"]["aws_db_instance"]["db"];
        assert_eq!(db["allocated_storage"], json!(100));
        assert_eq!(db["port"], json!(3306));
    }

    // A .tftpl is template text, not standalone HCL: it stays on the YAML/JSON
    // path. Native-HCL content that projects cleanly as a .tf therefore does NOT
    // project under a .tftpl name — it surfaces parse_error like any non-YAML/JSON
    // content — which is exactly the routing rider.
    #[test]
    fn tftpl_is_not_hcl_projected() {
        // Real Terraform template text (a %{ for } directive) — not standalone
        // HCL. It stays on the YAML/JSON path, where it does not parse, so it
        // surfaces parse_error rather than an HCL-projected resource object.
        let content = "%{ for ip in ips ~}\n${ip}:${port}\n%{ endfor ~}\n";
        let input = one_artifact_input("infra/terraform/user_data.tftpl", content);
        let doc = build_input(&input);
        let artifact = &doc["artifacts"][0];
        assert_eq!(
            artifact["documents"].as_array().unwrap().len(),
            0,
            ".tftpl is not HCL-projected, so no resource documents: {artifact}"
        );
        assert!(
            artifact["parse_error"].is_string(),
            ".tftpl template text surfaces parse_error: {artifact}"
        );
    }

    // The routing predicate itself: only a bare .tf is native HCL.
    #[test]
    fn native_hcl_predicate() {
        assert!(is_native_hcl("main.tf"));
        assert!(is_native_hcl("infra/terraform/main.tf"));
        assert!(!is_native_hcl("main.tf.json"));
        assert!(!is_native_hcl("vars.auto.tfvars.json"));
        assert!(!is_native_hcl("user_data.tftpl"));
        assert!(!is_native_hcl("compose.yaml"));
    }
}
