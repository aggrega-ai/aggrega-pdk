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
//! Schemas are read from the `aggrega-schemas` repository checked out beside
//! this one, at `../../aggrega-schemas/json/` relative to this crate root. A
//! run that could not read them has verified nothing, so these tests **refuse** with a clear
//! diagnostic rather than passing quietly. Where the schemas are genuinely
//! absent — a standalone checkout of this kit, or a packaging test — opt out
//! with:
//!
//! ```text
//! AGGREGA_SKIP_SCHEMA_TESTS=1 cargo test
//! ```
//!
//! Each affected test then reports that it was skipped and why.

use std::io::Write;
use std::path::{Path, PathBuf};

use aggrega_pdk_rust::{
    CLIRunInput, CompilationContext, DerivedNameVerdict, Diagnostic, IREdge, IRGraph, IRMetadata,
    IRNode, IRProvenance, IRSpec, LinterInput, LinterOutput, MutatorGraph, MutatorInput,
    MutatorNode, MutatorOutput, NameBoundInput, NameBoundOutput, OrderedMap, Policies, PolicyInput,
    PolicyOutput, PrecalculatedPath, API_VERSION,
};

/// Names the environment switch that opts out of the tests which validate
/// against the JSON Schemas published alongside this kit. Set it to any
/// non-empty value where those schemas are genuinely absent — a standalone
/// checkout of this kit, or a packaging test — and each affected test reports
/// that it was skipped and why.
const SKIP_SCHEMA_TESTS_ENV: &str = "AGGREGA_SKIP_SCHEMA_TESTS";

/// schema_dir resolves the absolute path to the sibling aggrega-schemas
/// repository's json directory. Resolution only: whether that path can be read
/// is decided in one place, by `schema_path_or_reason`.
fn schema_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("..")
        .join("..")
        .join("aggrega-schemas")
        .join("json")
}

/// schema_path_or_reason resolves the absolute path to a named JSON Schema
/// file, or returns the reason the schemas cannot be read.
///
/// This is the one place that decides reachability, so every test reaches the
/// same verdict on the same condition.
fn schema_path_or_reason(name: &str) -> Result<PathBuf, String> {
    let dir = schema_dir();
    if !dir.is_dir() {
        return Err(format!("schemas not reachable at {}", dir.display()));
    }
    let path = dir.join(name);
    if !path.exists() {
        return Err(format!("schema {name} not reachable at {}", path.display()));
    }
    Ok(path)
}

/// skip_schema_tests reports whether the opt-out switch is set to a non-empty
/// value. A value is not parsed: any non-empty setting opts out, so no spelling
/// of "yes" can be read as "no" and quietly leave the check running.
fn skip_schema_tests() -> bool {
    std::env::var_os(SKIP_SCHEMA_TESTS_ENV).is_some_and(|value| !value.is_empty())
}

/// announce_skip states, on the process stderr, that a test was skipped and why.
///
/// Three details are deliberate.
///
/// It bypasses the print macros. The test runner captures those and shows them
/// only for failures, so a skip announced through them would be invisible on a
/// passing run — exactly the silent outcome the opt-out exists to avoid.
/// Writing to the stderr handle is not captured, so the reason is stated
/// without asking the run for extra output.
///
/// It names the test. The runner has no result label for a test that decided
/// at run time not to check anything, so it reports one as passing; naming the
/// test, and saying plainly that it verified nothing, is what keeps that from
/// being read as a verified pass.
///
/// It writes the whole line in a single call. Tests run concurrently, so a
/// line assembled from several writes interleaves with the runner's own
/// progress output and with other tests' messages, and the reason arrives
/// shredded across lines.
fn announce_skip(reason: &str) {
    let thread = std::thread::current();
    let test = thread.name().unwrap_or("test").to_string();
    let line = format!(
        "SKIP {test}: {reason}; {SKIP_SCHEMA_TESTS_ENV} is set — this test verified nothing.\n"
    );
    let _ = std::io::stderr().write_all(line.as_bytes());
}

/// require_schema resolves a named JSON Schema, or ends the calling test.
///
/// The default is refusal. A harness that could not reach its inputs has
/// verified nothing, so reporting that is strictly more informative than
/// passing quietly — and a suite that skips by default cannot tell "everything
/// agrees" apart from "nothing was checked".
///
/// Setting `AGGREGA_SKIP_SCHEMA_TESTS` turns the refusal into a skip that
/// states the reason it was taken, so a deliberate run without the schemas
/// stays clean while an accidental one stays loud.
///
/// It expands to an early `return`, so it is written at the top of a test
/// rather than inside a helper.
macro_rules! require_schema {
    ($name:expr) => {
        match schema_path_or_reason($name) {
            Ok(path) => path,
            Err(reason) => {
                if skip_schema_tests() {
                    announce_skip(&reason);
                    return;
                }
                panic!(
                    "{reason}: these tests validate against the JSON Schemas published \
                     alongside this kit and verify nothing without them. Check out the \
                     aggrega-schemas repository beside this one (aggrega-pdk/ and \
                     aggrega-schemas/ as siblings), or set {SKIP_SCHEMA_TESTS_ENV}=1 to skip them."
                );
            }
        }
    };
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
fn schema_declared_abi_version(path: &Path) -> String {
    let value: serde_json::Value =
        serde_json::from_slice(&std::fs::read(path).expect("read describe schema"))
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
    let path = require_schema!("wasm-describe-v1alpha1.schema.json");
    let want = schema_declared_abi_version(&path);
    assert_eq!(
        API_VERSION, want,
        "API_VERSION does not match the abi_version the descriptor schema declares; \
         this constant is generated from that declaration, so a mismatch means the \
         generated const is stale — regenerate it"
    );
}

#[test]
fn cli_run_input_round_trip_json_schema() {
    let schema = require_schema!("wasm-cli-v1alpha1.schema.json");

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
    let schema = require_schema!("wasm-mutator-v1alpha1.schema.json");

    let mut labels = OrderedMap::new();
    labels.insert("team".to_string(), "payments".to_string());

    // The node's declared values, each carried as the verbatim JSON of its value.
    let mut values = OrderedMap::new();
    values.insert(
        "extensions".to_string(),
        serde_json::value::RawValue::from_string(r#"["vector"]"#.to_string())
            .expect("raw value"),
    );

    let fixture = MutatorInput {
        compilation_context: CompilationContext {
            release_name: "payments".to_string(),
            environment: "prod".to_string(),
        },
        graph: MutatorGraph {
            nodes: OrderedMap::from_iter([(
                "payments-api".to_string(),
                MutatorNode {
                    component: "core/web-service".to_string(),
                    resolved_version: "1.4.2".to_string(),
                    labels,
                    values,
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
    let schema = require_schema!("wasm-mutator-v1alpha1.schema.json");

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
    let schema = require_schema!("wasm-policy-v1alpha1.schema.json");

    let mut labels = OrderedMap::new();
    labels.insert("team".to_string(), "payments".to_string());

    let fixture = PolicyInput {
        graph: IRGraph {
            api_version: "aggrega.ai/v1alpha1".to_string(),
            kind: "IR".to_string(),
            metadata: IRMetadata {
                aggregate_name: "payments".to_string(),
            },
            spec: IRSpec {
                nodes: OrderedMap::from_iter([(
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
                values_mapping: OrderedMap::new(),
                infra_mapping: OrderedMap::new(),
                feature_values_mapping: OrderedMap::new(),
                boundary_features: OrderedMap::new(),
                provenance: IRProvenance {
                    schema_version: "v1alpha1".to_string(),
                    compiler_version: "test".to_string(),
                    aggregate_version: "1.0.0".to_string(),
                    manifest_version: "1.0.0".to_string(),
                    release_version: "1.0.0".to_string(),
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
    let schema = require_schema!("wasm-policy-v1alpha1.schema.json");

    let fixture = PolicyOutput {
        results: vec![
            Diagnostic {
                severity: "halt".to_string(),
                message: "version floor violation: payments-db < 15.0.0".to_string(),
                ..Default::default()
            },
            Diagnostic {
                severity: "info".to_string(),
                message: "all labels present".to_string(),
                ..Default::default()
            },
        ],
        ..Default::default()
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize PolicyOutput");
    validate(&schema, &serialised).expect("schema validation");

    let round: PolicyOutput = serde_json::from_value(serialised).expect("deserialize PolicyOutput");
    assert_eq!(
        serde_json::to_string(&fixture).unwrap(),
        serde_json::to_string(&round).unwrap()
    );
}

#[test]
fn linter_input_round_trip_json_schema() {
    let schema = require_schema!("wasm-linter-v1alpha1.schema.json");

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
        // Populated rather than left None, so the field actually crosses
        // serialize -> schema-validate -> deserialize. An omitted optional field
        // round-trips vacuously: the assertion would hold for a field the codec
        // does not carry at all.
        implemented_component: Some(
            serde_json::value::RawValue::from_string(
                r#"{"kind":"Component","spec":{"schema":{"provides":{"endpoint":{"type":"string"}}}}}"#
                    .into(),
            )
            .unwrap(),
        ),
        // The two deployment fields are populated for the same reason: each then
        // crosses serialize -> schema-validate -> deserialize, so a schema that did
        // not admit the Release, or a components value of the wrong shape, fails here.
        release: Some(
            serde_json::value::RawValue::from_string(
                r#"{"kind":"Release","spec":{"aggregate":"./aggregate.yaml"}}"#.into(),
            )
            .unwrap(),
        ),
        components: OrderedMap::from_iter([(
            "router".to_string(),
            serde_json::value::RawValue::from_string(
                r#"{"kind":"Component","spec":{"featuresSchema":{"api_key":{"sensitive":true}}}}"#.into(),
            )
            .unwrap(),
        )]),
        source_path: "core/compute/api-server/api-server.yaml".to_string(),
        policies: Policies {
            allow_dynamic_refs: false,
            allow_id_overrides: false,
            allow_direct_x_aggrega_extensions: false,
            allow_aggrega_ai_prefix: false,
            allow_nonstandard_names: false,
            allow_category_mismatches: false,
            allow_undeclared_recipe_provides: false,
            allow_connection_credential_mismatches: false,
        },
    };

    let serialised = serde_json::to_value(&fixture).expect("serialize LinterInput");
    validate(&schema, &serialised).expect("schema validation");
    let _round: LinterInput = serde_json::from_value(serialised).expect("deserialize LinterInput");
}

#[test]
fn linter_output_round_trip_json_schema() {
    let schema = require_schema!("wasm-linter-v1alpha1.schema.json");

    let fixture = LinterOutput {
        findings: vec![
            Diagnostic {
                rule_id: "schema-evolution".to_string(),
                severity: "halt".to_string(),
                classification: "MAJOR".to_string(),
                message: "field spec.schema.values.port removed".to_string(),
                ..Default::default()
            },
            Diagnostic {
                rule_id: "unknown_rule".to_string(),
                severity: "halt".to_string(),
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

/// The three states a name-bound verdict's bound can be in are distinguishable
/// on the wire: present with a value, present as zero, and absent.
///
/// The distinction is the whole reason the field is optional rather than an
/// integer with a sentinel. Absent says length is not what the target objected
/// to — either it imposes none, or the objection is one no shortening repairs.
/// Zero says no identity of any length reaches an acceptable name. A host that
/// could not tell them apart would narrow a name toward a bound that was never
/// the obstacle, and would do it until it ran out of passes.
///
/// Serialising is where the two could collapse: a bound written as a plain
/// integer would emit `0` for the absent case, and nothing downstream could
/// recover the difference. So the assertion is over the emitted TEXT and not
/// only over the round-tripped value.
#[test]
fn name_bound_output_distinguishes_an_absent_bound_from_zero() {
    let schema = require_schema!("wasm-provider-v1alpha1.schema.json");

    let fixture = NameBoundOutput {
        names: vec![
            DerivedNameVerdict {
                kind: "release name".to_string(),
                usable: false,
                bound: Some(31),
                reason: "a release name is at most 53 characters".to_string(),
            },
            DerivedNameVerdict {
                kind: "logical id".to_string(),
                usable: false,
                bound: Some(0),
                reason: "this target accepts no name of this kind".to_string(),
            },
            DerivedNameVerdict {
                kind: "service name".to_string(),
                usable: false,
                bound: None,
                reason: "the first character is one the alphabet does not admit".to_string(),
            },
        ],
        error: String::new(),
        condition_code: String::new(),
        condition_short_code: String::new(),
    };

    let value = serde_json::to_value(&fixture).expect("serialise NameBoundOutput");
    if let Err(e) = validate(&schema, &value) {
        panic!("schema validation failed for NameBoundOutput fixture: {e}");
    }

    let raw = serde_json::to_string(&fixture).expect("marshal NameBoundOutput");
    assert!(
        raw.contains("\"bound\":0"),
        "a zero bound must reach the wire as a stated length; got {raw}"
    );
    let round: NameBoundOutput = serde_json::from_str(&raw).expect("unmarshal NameBoundOutput");
    assert_eq!(round.names[0].bound, Some(31));
    assert_eq!(
        round.names[1].bound,
        Some(0),
        "a zero bound must survive the round trip as a stated length"
    );
    assert_eq!(
        round.names[2].bound, None,
        "an absent bound must survive the round trip as absent, not as zero"
    );
}

/// A guest reads the proposed identity and whether a person wrote it. The flag
/// is carried so a guest can phrase its reason for the reader who can act on it;
/// what a refusal DOES is the host's decision, not the guest's.
#[test]
fn name_bound_input_round_trip_json_schema() {
    let schema = require_schema!("wasm-provider-v1alpha1.schema.json");

    let fixture = NameBoundInput {
        proposed: "payments-api".to_string(),
        authored: true,
    };

    let value = serde_json::to_value(&fixture).expect("serialise NameBoundInput");
    if let Err(e) = validate(&schema, &value) {
        panic!("schema validation failed for NameBoundInput fixture: {e}");
    }

    let raw = serde_json::to_string(&fixture).expect("marshal NameBoundInput");
    let round: NameBoundInput = serde_json::from_str(&raw).expect("unmarshal NameBoundInput");
    assert_eq!(fixture.proposed, round.proposed);
    assert!(round.authored);
}
