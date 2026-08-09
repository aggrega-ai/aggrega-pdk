// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Schema-validation round-trip tests for the Aggrega Rust PDK.
//!
//! Each test constructs a fixture value, serialises it, validates the
//! serialised value against the corresponding JSON Schema in the sibling
//! `aggrega-schemas` repository, and deserialises it back.
//!
//! The mutator, linter, and policy socket types are all generated
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

use std::path::{Path, PathBuf};

use aggrega_pdk_rust::{
    CLIRunInput, CompilationContext, Diagnostic, IREdge, IRGraph, IRMetadata, IRNode, IRProvenance,
    IRSpec, LinterInput, LinterOutput, MutatorGraph, MutatorInput, MutatorNode, MutatorOutput,
    Policies, PolicyInput, PolicyOutput,
    PrecalculatedPath, API_VERSION,
};
use std::collections::BTreeMap;

/// schema_dir resolves the absolute path to the sibling aggrega-schemas
/// repository's json directory. Panics if the directory is unreachable — see the
/// file-level comment for the rationale and the disconnected-dev opt-out via
/// `cargo test -- --skip round_trip`.
fn schema_dir() -> PathBuf {
    let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let dir = manifest_dir
        .join("..")
        .join("..")
        .join("aggrega-schemas")
        .join("json");
    assert!(
        dir.is_dir(),
        "schemas not reachable at {}: this test requires the umbrella checkout layout \
         (aggrega-pdk/ and aggrega-schemas/ as siblings). Disconnected dev opts out via \
         `cargo test -- --skip round_trip`.",
        dir.display()
    );
    dir
}

/// schema_path resolves the absolute path to a JSON Schema file in the
/// sibling aggrega-schemas repository. Panics if the file is unreachable —
/// see the file-level comment for the rationale and the disconnected-dev
/// opt-out via `cargo test -- --skip round_trip`.
fn schema_path(name: &str) -> PathBuf {
    let path = schema_dir().join(name);
    assert!(
        path.exists(),
        "schema {name} not reachable at {}: this test requires the umbrella checkout layout \
         (aggrega-pdk/ and aggrega-schemas/ as siblings). Disconnected dev opts out via \
         `cargo test -- --skip round_trip`.",
        path.display()
    );
    path
}

/// validate validates `instance` against the JSON Schema at `path`, with every
/// schema in the sibling json directory registered alongside it. The registration
/// set is derived by reading that directory, so a cross-file $ref resolves without
/// a network loader and without any per-caller list of referenced schemas to keep
/// in step with the schemas themselves — an unregistered ref target does not
/// produce a wrong answer, it aborts compilation and masks every assertion that
/// follows.
///
/// Each schema registers under its $id when it declares one, falling back to its
/// file name: a relative cross-file $ref resolves against the referring schema's
/// base URI, which is its absolute $id URL, so the target must be reachable under
/// that URL rather than under its file name. The schema at `path` is compiled by
/// the same identity it was registered under.
fn validate(path: &Path, instance: &serde_json::Value) -> Result<(), String> {
    let dir = schema_dir();
    let mut names: Vec<String> = std::fs::read_dir(&dir)
        .map_err(|e| format!("read schema dir {}: {e}", dir.display()))?
        .filter_map(|entry| entry.ok())
        .map(|entry| entry.file_name().to_string_lossy().into_owned())
        .filter(|name| name.ends_with(".schema.json"))
        .collect();
    names.sort();

    let primary = path
        .file_name()
        .map(|n| n.to_string_lossy().into_owned())
        .ok_or_else(|| format!("schema path {} has no file name", path.display()))?;

    let mut compiler = boon::Compiler::new();
    let mut primary_id: Option<String> = None;
    for name in &names {
        let value: serde_json::Value = serde_json::from_slice(
            &std::fs::read(dir.join(name)).map_err(|e| format!("read schema {name}: {e}"))?,
        )
        .map_err(|e| format!("parse schema {name}: {e}"))?;
        let id = value
            .get("$id")
            .and_then(|v| v.as_str())
            .map(|s| s.to_string())
            .unwrap_or_else(|| name.clone());
        if *name == primary {
            primary_id = Some(id.clone());
        }
        compiler
            .add_resource(&id, value)
            .map_err(|e| format!("add schema {id}: {e}"))?;
    }
    let primary_id =
        primary_id.ok_or_else(|| format!("schema {primary} not found in {}", dir.display()))?;

    let mut schemas = boon::Schemas::new();
    let idx = compiler
        .compile(&primary_id, &mut schemas)
        .map_err(|e| format!("compile schema: {e}"))?;
    schemas
        .validate(instance, idx)
        .map_err(|e| format!("validation failed: {e}"))
}

/// schema_declared_abi_version reads the plugin ABI version out of the descriptor
/// schema in the sibling aggrega-schemas repository — the single-valued const the
/// schema declares for a descriptor's `abi_version`. Reading the declaration
/// rather than repeating its value is the whole point: a literal here would pin
/// this kit's constant against a second copy living in this same repository,
/// which is the divergence that generating the constant from the schema exists to
/// remove.
fn schema_declared_abi_version() -> String {
    let path = schema_path("wasm-describe-v1alpha1.schema.json");
    let value: serde_json::Value =
        serde_json::from_slice(&std::fs::read(&path).expect("read describe schema"))
            .expect("parse describe schema");
    let declared = value
        .pointer("/$defs/PluginDescriptor/properties/abi_version/const")
        .and_then(|v| v.as_str());
    // A missing read means the declaration moved, not that the two versions
    // agree. Panicking keeps a silent absence from being reported as a match.
    match declared {
        Some(s) => s.to_string(),
        None => panic!(
            "no abi_version const found at /$defs/PluginDescriptor/properties in {}: \
             the declaration moved, so this assertion is no longer reading it",
            path.display()
        ),
    }
}

/// Checks this kit's exported constant against the schema that declares the
/// version, which is the one artifact upstream of every language's copy.
#[test]
fn api_version_matches_schema_declaration() {
    let want = schema_declared_abi_version();
    assert_eq!(
        API_VERSION, want,
        "API_VERSION does not match the abi_version the descriptor schema declares; \
         this constant is generated from that declaration, so a mismatch means the \
         generated const is stale — regenerate it"
    );
}

#[test]
fn cli_run_input_round_trip_json_schema() {
    let schema = schema_path("wasm-cli-v1alpha1.schema.json");

    let fixture = CLIRunInput {
        command: "deploy".to_string(),
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
        sources: Vec::new(),
        settings: None,
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
        ..Default::default()
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize MutatorOutput");
    validate(&schema, &serialised).expect("schema validation");
}

#[test]
fn policy_input_round_trip_json_schema() {
    // Under b-full the policy input carries the full IR graph; the schema
    // references ir-v1alpha1 for the graph field, so both schemas are registered.
    let schema = schema_path("wasm-policy-v1alpha1.schema.json");

    let mut labels = BTreeMap::new();
    labels.insert("team".to_string(), "payments".to_string());

    let fixture = PolicyInput {
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
                    socket_declaration: "declared".to_string(),
                    ..Default::default()
                }],
                feature_markers: vec![],
                values_mapping: BTreeMap::new(),
                infra_mapping: BTreeMap::new(),
                feature_values_mapping: BTreeMap::new(),
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
        sources: vec![],
        settings: None,
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize PolicyInput");
    validate(&schema, &serialised).expect("schema validation");

    // The abi types do not derive PartialEq (RawValue lacks it); compare the
    // re-serialized wire instead.
    let round: PolicyInput =
        serde_json::from_value(serialised.clone()).expect("deserialize PolicyInput");
    assert_eq!(
        serde_json::to_string(&fixture).unwrap(),
        serde_json::to_string(&round).unwrap()
    );
}

#[test]
fn policy_output_round_trip_json_schema() {
    let schema = schema_path("wasm-policy-v1alpha1.schema.json");

    let fixture = PolicyOutput {
        results: vec![
            Diagnostic {
                severity: "deny".to_string(),
                message: "version floor violation: payments-db < 15.0.0".to_string(),
                ..Default::default()
            },
            Diagnostic {
                severity: "audit".to_string(),
                message: "all labels present".to_string(),
                ..Default::default()
            },
        ],
        ..Default::default()
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize PolicyOutput");
    validate(&schema, &serialised).expect("schema validation");

    let round: PolicyOutput =
        serde_json::from_value(serialised).expect("deserialize PolicyOutput");
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
        source_path: "core/compute/api-server/api-server.yaml".to_string(),
        policies: Policies {
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
            Diagnostic {
                rule_id: "schema-evolution".to_string(),
                severity: "error".to_string(),
                classification: "MAJOR".to_string(),
                message: "field spec.schema.values.port removed".to_string(),
                ..Default::default()
            },
            Diagnostic {
                rule_id: "unknown_rule".to_string(),
                severity: "error".to_string(),
                classification: String::new(),
                message: "rule_id not implemented".to_string(),
                ..Default::default()
            },
        ],
        ..Default::default()
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize LinterOutput");
    validate(&schema, &serialised).expect("schema validation");
    let _round: LinterOutput =
        serde_json::from_value(serialised).expect("deserialize LinterOutput");
}
