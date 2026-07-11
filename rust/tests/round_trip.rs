// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Schema-validation round-trip tests for the Aggrega Rust PDK.
//!
//! Each test constructs a fixture value, serialises it, validates the
//! serialised value against the corresponding JSON Schema in the sibling
//! `aggrega-schemas` repository, and deserialises it back.
//!
//! The mutator, linter, and policy-validator socket types are all generated
//! into the portable `aggrega-abi` crate (re-exported by this PDK) and their
//! cross-language wire parity is pinned by the parity tests in `aggrega-abi`.
//! The tests here are a schema-validation smoke over the re-exported socket
//! types so a PDK-authored value is confirmed to validate against the
//! published JSON Schemas.
//!
//! Schemas live alongside the umbrella checkout at
//! `../../aggrega-schemas/json/` relative to this crate root. Tests
//! **panic** with a clear diagnostic when the schema file is unreachable
//! rather than silently returning. Disconnected dev opts out via:
//!
//! ```text
//! cargo test --workspace -- --skip round_trip
//! ```

use std::path::PathBuf;

use aggrega_pdk_rust::{
    CLIRunInput, CompilationContext, IREdge, IRGraph, IRMetadata, IRNode, IRProvenance, IRSpec,
    LinterFinding,
    LinterInput, LinterOutput, MutatorGraph, MutatorInput, MutatorNode, MutatorOutput,
    NarrowingExemption, Policies, PolicyResult, PolicyValidatorInput, PolicyValidatorOutput,
    PrecalculatedPath, API_VERSION,
};
use std::collections::BTreeMap;

/// schema_path resolves the absolute path to a JSON Schema file in the
/// sibling aggrega-schemas repository. Panics if the file is unreachable —
/// see the file-level comment for the rationale and the disconnected-dev
/// opt-out via `cargo test -- --skip round_trip`.
fn schema_path(name: &str) -> PathBuf {
    let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let path = manifest_dir
        .join("..")
        .join("..")
        .join("aggrega-schemas")
        .join("json")
        .join(name);
    assert!(
        path.exists(),
        "schema {name} not reachable at {}: this test requires the umbrella checkout layout \
         (aggrega-pdk/ and aggrega-schemas/ as siblings). Disconnected dev opts out via \
         `cargo test -- --skip round_trip`.",
        path.display()
    );
    path
}

/// validate validates `instance` against the JSON Schema at `path`.
fn validate(path: &PathBuf, instance: &serde_json::Value) -> Result<(), String> {
    let schema_bytes = std::fs::read(path).map_err(|e| format!("read schema: {e}"))?;
    let schema_value: serde_json::Value =
        serde_json::from_slice(&schema_bytes).map_err(|e| format!("parse schema: {e}"))?;

    let mut compiler = boon::Compiler::new();
    compiler
        .add_resource("schema.json", schema_value)
        .map_err(|e| format!("add schema: {e}"))?;
    let mut schemas = boon::Schemas::new();
    let schema_idx = compiler
        .compile("schema.json", &mut schemas)
        .map_err(|e| format!("compile schema: {e}"))?;

    schemas
        .validate(instance, schema_idx)
        .map_err(|e| format!("validation failed: {e}"))
}

/// validate_with_ref validates an instance against `path`, registering the
/// referenced schema at `ref_path` under its $id so a cross-file $ref (the
/// policy schema's graph referencing ir-v1alpha1) resolves without a network
/// loader.
fn validate_with_ref(path: &PathBuf, ref_path: &PathBuf, instance: &serde_json::Value) -> Result<(), String> {
    let main_value: serde_json::Value = serde_json::from_slice(
        &std::fs::read(path).map_err(|e| format!("read schema: {e}"))?,
    )
    .map_err(|e| format!("parse schema: {e}"))?;
    let ref_value: serde_json::Value = serde_json::from_slice(
        &std::fs::read(ref_path).map_err(|e| format!("read ref schema: {e}"))?,
    )
    .map_err(|e| format!("parse ref schema: {e}"))?;
    let ref_id = ref_value
        .get("$id")
        .and_then(|v| v.as_str())
        .ok_or("ref schema missing $id")?
        .to_string();
    let main_id = main_value
        .get("$id")
        .and_then(|v| v.as_str())
        .ok_or("main schema missing $id")?
        .to_string();

    let mut compiler = boon::Compiler::new();
    compiler
        .add_resource(&ref_id, ref_value)
        .map_err(|e| format!("add ref schema: {e}"))?;
    compiler
        .add_resource(&main_id, main_value)
        .map_err(|e| format!("add schema: {e}"))?;
    let mut schemas = boon::Schemas::new();
    let idx = compiler
        .compile(&main_id, &mut schemas)
        .map_err(|e| format!("compile schema: {e}"))?;
    schemas
        .validate(instance, idx)
        .map_err(|e| format!("validation failed: {e}"))
}

#[test]
fn api_version_v1alpha1_contract_floor() {
    assert_eq!(API_VERSION, "v1alpha1");
}

#[test]
fn cli_run_input_round_trip_json_schema() {
    let schema = schema_path("wasm-cli-v1alpha1.schema.json");

    let fixture = CLIRunInput {
        args: vec![
            "deploy".to_string(),
            "--env".to_string(),
            "prod".to_string(),
            "payments".to_string(),
        ],
    };

    let value = serde_json::to_value(&fixture).expect("serialise CLIRunInput");
    if let Err(e) = validate(&schema, &value) {
        panic!("schema validation failed for CLIRunInput fixture: {e}");
    }

    let raw = serde_json::to_string(&fixture).expect("marshal CLIRunInput");
    let round: CLIRunInput = serde_json::from_str(&raw).expect("unmarshal CLIRunInput");
    assert_eq!(fixture.args, round.args);
}

#[test]
fn mutator_input_round_trip_json_schema() {
    let schema = schema_path("wasm-mutator-v1alpha1.schema.json");

    let mut labels = std::collections::BTreeMap::new();
    labels.insert("team".to_string(), "payments".to_string());

    let fixture = MutatorInput {
        compilation_context: CompilationContext {
            release_name: "payments".to_string(),
            environment: "prod".to_string(),
        },
        graph: MutatorGraph {
            nodes: std::collections::BTreeMap::from([(
                "payments-api".to_string(),
                MutatorNode {
                    component: "core/web-service".to_string(),
                    resolved_version: "1.4.2".to_string(),
                    labels,
                },
            )]),
        },
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize MutatorInput");
    validate(&schema, &serialised).expect("schema validation");
    let _round: MutatorInput =
        serde_json::from_value(serialised).expect("deserialize MutatorInput");
}

#[test]
fn mutator_output_round_trip_json_schema() {
    let schema = schema_path("wasm-mutator-v1alpha1.schema.json");

    let patch =
        serde_json::value::RawValue::from_string(r#"{"op":"add","path":"/x","value":"y"}"#.into())
            .unwrap();
    let fixture = MutatorOutput {
        patches: vec![patch],
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize MutatorOutput");
    validate(&schema, &serialised).expect("schema validation");
}

#[test]
fn policy_validator_input_round_trip_json_schema() {
    // Under b-full the policy input carries the full IR graph; the schema
    // references ir-v1alpha1 for the graph field, so both schemas are registered.
    let schema = schema_path("wasm-policy-validator-v1alpha1.schema.json");
    let ir_schema = schema_path("ir-v1alpha1.schema.json");

    let mut labels = BTreeMap::new();
    labels.insert("team".to_string(), "payments".to_string());

    let fixture = PolicyValidatorInput {
        graph: IRGraph {
            api_version: "aggrega.ai/v1alpha1".to_string(),
            kind: "IR".to_string(),
            metadata: IRMetadata {
                aggregate_name: "payments".to_string(),
            },
            spec: IRSpec {
                nodes: BTreeMap::from([(
                    "payments-api".to_string(),
                    IRNode {
                        component: "core/web-service".to_string(),
                        resolved_version: "1.4.2".to_string(),
                        labels,
                        ..Default::default()
                    },
                )]),
                edges: vec![IREdge {
                    source: "payments-api".to_string(),
                    target: "payments-db".to_string(),
                    socket: "databases".to_string(),
                    ..Default::default()
                }],
                feature_markers: vec![],
                values_mapping: BTreeMap::new(),
                infra_mapping: BTreeMap::new(),
                feature_values_mapping: BTreeMap::new(),
                secrets: BTreeMap::new(),
                boundary_features: BTreeMap::new(),
                provenance: IRProvenance {
                    schema_version: "v1alpha1".to_string(),
                    compiler_version: "test".to_string(),
                    aggregate_version: "1.0.0".to_string(),
                    manifest_version: "1.0.0".to_string(),
                    release_version: "1.0.0".to_string(),
                    source_identifiers: vec!["x".to_string()],
                },
            },
        },
        precalculated_paths: vec![PrecalculatedPath {
            path_id: "path-0001".to_string(),
            nodes: vec!["payments-api".to_string(), "payments-db".to_string()],
            attached_features: vec!["feature/observability".to_string()],
        }],
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize PolicyValidatorInput");
    validate_with_ref(&schema, &ir_schema, &serialised).expect("schema validation");

    // The abi types do not derive PartialEq (RawValue lacks it); compare the
    // re-serialized wire instead.
    let round: PolicyValidatorInput =
        serde_json::from_value(serialised.clone()).expect("deserialize PolicyValidatorInput");
    assert_eq!(
        serde_json::to_string(&fixture).unwrap(),
        serde_json::to_string(&round).unwrap()
    );
}

#[test]
fn policy_validator_output_round_trip_json_schema() {
    let schema = schema_path("wasm-policy-validator-v1alpha1.schema.json");
    let ir_schema = schema_path("ir-v1alpha1.schema.json");

    let fixture = PolicyValidatorOutput {
        results: vec![
            PolicyResult {
                pass: true,
                message: "all labels present".to_string(),
            },
            PolicyResult {
                pass: false,
                message: "version floor violation: payments-db < 15.0.0".to_string(),
            },
        ],
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize PolicyValidatorOutput");
    validate_with_ref(&schema, &ir_schema, &serialised).expect("schema validation");

    let round: PolicyValidatorOutput =
        serde_json::from_value(serialised).expect("deserialize PolicyValidatorOutput");
    assert_eq!(
        serde_json::to_string(&fixture).unwrap(),
        serde_json::to_string(&round).unwrap()
    );
}

#[test]
fn linter_input_round_trip_json_schema() {
    let schema = schema_path("wasm-linter-v1alpha1.schema.json");

    let fixture = LinterInput {
        rule_id: "schema-evolution".to_string(),
        entity_kind: "Component".to_string(),
        proposed: serde_json::value::RawValue::from_string(
            r#"{"apiVersion":"aggrega.ai/v1alpha1","kind":"Component"}"#.into(),
        )
        .unwrap(),
        previous: Some(
            serde_json::value::RawValue::from_string(r#"{"kind":"Component"}"#.into()).unwrap(),
        ),
        parent: None,
        source_path: "core/compute/api-server/api-server.yaml".to_string(),
        policies: Policies {
            enforce_monotonic_narrowing: true,
            enforce_immutability: true,
            narrowing_exemptions: vec![NarrowingExemption {
                entity: "core/legacy-api".to_string(),
                paths: vec!["spec.schema.values.timeout".to_string()],
                reason: "documented exemption pending refactor".to_string(),
            }],
            allow_dynamic_refs: false,
            allow_id_overrides: false,
            allow_direct_x_aggrega_extensions: false,
            allow_aggrega_ai_prefix: false,
            allow_nonstandard_names: false,
            allow_category_mismatches: false,
        },
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize LinterInput");
    validate(&schema, &serialised).expect("schema validation");
    let _round: LinterInput = serde_json::from_value(serialised).expect("deserialize LinterInput");
}

#[test]
fn linter_output_round_trip_json_schema() {
    let schema = schema_path("wasm-linter-v1alpha1.schema.json");

    let fixture = LinterOutput {
        findings: vec![
            LinterFinding {
                rule_id: "schema-evolution".to_string(),
                severity: "error".to_string(),
                classification: "MAJOR".to_string(),
                message: "field spec.schema.values.port removed".to_string(),
            },
            LinterFinding {
                rule_id: "unknown_rule".to_string(),
                severity: "error".to_string(),
                classification: String::new(),
                message: "rule_id not implemented".to_string(),
            },
        ],
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize LinterOutput");
    validate(&schema, &serialised).expect("schema validation");
    let _round: LinterOutput =
        serde_json::from_value(serialised).expect("deserialize LinterOutput");
}
