// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Round-trip mirroring tests for the Aggrega PDK socket types.
//!
//! Each test:
//!  1. Constructs a fixture value of the PDK type.
//!  2. Serialises via `serde_json::to_value` to a `serde_json::Value`.
//!  3. Validates against the corresponding JSON Schema in the sibling
//!     `aggrega-schemas` repository.
//!  4. Deserialises back to a Rust value via `serde_json::from_value`.
//!  5. Asserts equality.
//!
//! Schemas live alongside the umbrella checkout at
//! `../../aggrega-schemas/json-schemas/` relative to this crate root.
//! Tests **panic** with a clear diagnostic when the schema file is
//! unreachable rather than silently returning: Rust's stable test
//! framework has no dynamic-skip analog to Go's `t.Skipf`, and an
//! early `return` would mark the test PASSED in cargo's report,
//! masking a setup error. The panic surfaces FAILED instead.
//!
//! Disconnected dev (the PDK checked out without the umbrella) opts
//! out of the schema-dependent tests via:
//!
//! ```text
//! cargo test --workspace -- --skip round_trip
//! ```
//!
//! The umbrella's compiler CI always runs from an umbrella checkout
//! with the schemas present alongside; the panic path does not fire
//! there.

use std::collections::HashMap;
use std::path::PathBuf;

use aggrega_pdk_rust::{
    CompilationContext, LinterClassification, LinterFinding, LinterInput, LinterOutput,
    LinterSeverity, MutatorGraph, MutatorInput, MutatorNode, MutatorOutput, NarrowingExemption,
    Policies, PolicyEdge, PolicyFeatureEntry, PolicyGraph, PolicyNode, PolicyResult,
    PolicyValidatorInput, PolicyValidatorOutput, PrecalculatedPath, API_VERSION,
};

/// schema_path resolves the absolute path to a JSON Schema file in the
/// sibling aggrega-schemas repository. Panics if the file is
/// unreachable — see the file-level comment for the rationale and the
/// disconnected-dev opt-out via `cargo test -- --skip round_trip`.
fn schema_path(name: &str) -> PathBuf {
    let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let path = manifest_dir
        .join("..")
        .join("..")
        .join("aggrega-schemas")
        .join("json-schemas")
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

/// validate validates `instance` against the JSON Schema at `path` and
/// returns the validation outcome as a Result. Loads the schema once
/// per call (the test count is small enough that schema-compile cost
/// is negligible).
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

#[test]
fn api_version_v1alpha1_contract_floor() {
    assert_eq!(API_VERSION, "v1alpha1");
}

#[test]
fn mutator_input_round_trip_json_schema() {
    let schema = schema_path("wasm-mutator-v1alpha1.schema.json");

    let mut labels = HashMap::new();
    labels.insert("team".to_string(), "payments".to_string());

    let fixture = MutatorInput {
        compilation_context: CompilationContext {
            release_destination: "aws-prod/eks-main".to_string(),
            release_name: "payments".to_string(),
            environment: "prod".to_string(),
        },
        graph: MutatorGraph {
            nodes: HashMap::from([(
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

    let round: MutatorInput =
        serde_json::from_value(serialised).expect("deserialize MutatorInput");
    assert_eq!(fixture, round);
}

#[test]
fn mutator_output_round_trip_json_schema() {
    let schema = schema_path("wasm-mutator-v1alpha1.schema.json");

    let patch: serde_json::Value = serde_json::from_str(
        r#"[{"op":"add","path":"/nodes/payments-api/labels/compliance","value":"soc2"}]"#,
    )
    .unwrap();

    let fixture = MutatorOutput {
        patches: vec![patch],
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize MutatorOutput");
    validate(&schema, &serialised).expect("schema validation");
}

#[test]
fn policy_validator_input_round_trip_json_schema() {
    let schema = schema_path("wasm-policy-validator-v1alpha1.schema.json");

    let mut labels = HashMap::new();
    labels.insert("team".to_string(), "payments".to_string());

    let mut feature_values = HashMap::new();
    feature_values.insert(
        "tracing_enabled".to_string(),
        serde_json::Value::Bool(true),
    );

    let fixture = PolicyValidatorInput {
        graph: PolicyGraph {
            nodes: HashMap::from([(
                "payments-api".to_string(),
                PolicyNode {
                    component: "core/web-service".to_string(),
                    resolved_version: "1.4.2".to_string(),
                    labels,
                    features: HashMap::from([(
                        "observability".to_string(),
                        PolicyFeatureEntry {
                            r#type: "feature/observability".to_string(),
                            resolved_version: "2.0.1".to_string(),
                            values: feature_values,
                            infra: HashMap::new(),
                            origin: "explicit".to_string(),
                        },
                    )]),
                },
            )]),
            edges: vec![PolicyEdge {
                source: "payments-api".to_string(),
                target: "payments-db".to_string(),
                socket: "databases".to_string(),
                origin_feature: String::new(),
            }],
        },
        precalculated_paths: vec![PrecalculatedPath {
            path_id: "path-0001".to_string(),
            nodes: vec!["payments-api".to_string(), "payments-db".to_string()],
            attached_features: vec!["feature/observability".to_string()],
        }],
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize PolicyValidatorInput");
    validate(&schema, &serialised).expect("schema validation");

    let round: PolicyValidatorInput =
        serde_json::from_value(serialised).expect("deserialize PolicyValidatorInput");
    assert_eq!(fixture, round);
}

#[test]
fn policy_validator_output_round_trip_json_schema() {
    let schema = schema_path("wasm-policy-validator-v1alpha1.schema.json");

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
    validate(&schema, &serialised).expect("schema validation");

    let round: PolicyValidatorOutput =
        serde_json::from_value(serialised).expect("deserialize PolicyValidatorOutput");
    assert_eq!(fixture, round);
}

#[test]
fn linter_input_round_trip_json_schema() {
    let schema = schema_path("wasm-linter-v1alpha1.schema.json");

    let fixture = LinterInput {
        rule_id: "schema-evolution".to_string(),
        entity_kind: "Component".to_string(),
        proposed: serde_json::json!({
            "apiVersion": "aggrega.ai/v1alpha1",
            "kind": "Component",
            "metadata": {"name": "core/api-server", "version": "1.0.0"},
        }),
        previous: Some(serde_json::json!({
            "apiVersion": "aggrega.ai/v1alpha1",
            "kind": "Component",
            "metadata": {"name": "core/api-server", "version": "0.9.0"},
        })),
        policies: Some(Policies {
            enforce_monotonic_narrowing: true,
            enforce_immutability: true,
            narrowing_exemptions: vec![NarrowingExemption {
                entity: "core/legacy-api".to_string(),
                paths: vec!["spec.schema.values.timeout".to_string()],
                reason: "documented exemption pending refactor".to_string(),
            }],
            allow_dynamic_refs: false,
            allow_id_overrides: false,
            allow_non_pascal_schema_names: false,
            allow_direct_x_aggrega_extensions: false,
            allow_nonstandard_names: false,
        }),
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize LinterInput");
    validate(&schema, &serialised).expect("schema validation");

    let round: LinterInput =
        serde_json::from_value(serialised).expect("deserialize LinterInput");
    assert_eq!(fixture, round);
}

#[test]
fn linter_output_round_trip_json_schema() {
    let schema = schema_path("wasm-linter-v1alpha1.schema.json");

    let fixture = LinterOutput {
        findings: vec![
            LinterFinding {
                rule_id: "schema-evolution".to_string(),
                severity: LinterSeverity::Error,
                classification: Some(LinterClassification::Major),
                message: "field spec.schema.values.port removed".to_string(),
            },
            LinterFinding {
                rule_id: "unknown_rule".to_string(),
                severity: LinterSeverity::Error,
                classification: None,
                message: "rule_id \"not-implemented\" not implemented at this plugin version"
                    .to_string(),
            },
        ],
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize LinterOutput");
    validate(&schema, &serialised).expect("schema validation");

    let round: LinterOutput =
        serde_json::from_value(serialised).expect("deserialize LinterOutput");
    assert_eq!(fixture, round);
}
