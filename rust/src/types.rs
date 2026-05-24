// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Socket payload types — hand-authored mirrors of the JSON wire shape
//! the Aggrega Wasm Mutator and Policy Validator JSON Schemas describe.
//!
//! Each type derives `Serialize` + `Deserialize` so plugin authors can
//! round-trip them through `serde_json`. Field names match the JSON
//! Schema property names exactly (snake_case via Rust's default
//! identifier style; `serde`'s default rename is none, which is the
//! correct behaviour here because the Rust idiomatic identifier
//! already matches the wire name).
//!
//! The types are independent re-implementations: they share the JSON
//! wire format with the Aggrega compiler's canonical Go socket types
//! and with the Go and TypeScript PDK mirrors, but they are not
//! source-level translations. Drift between any mirror and the JSON
//! Schema is caught by the round-trip mirroring tests in `tests/`.

use std::collections::HashMap;

use serde::{Deserialize, Serialize};

/// The JSON payload the host hands a mutator plugin on each invocation.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct MutatorInput {
    pub compilation_context: CompilationContext,
    pub graph: MutatorGraph,
}

/// Release-level metadata the mutator reads when deciding which
/// compliance, FinOps, or governance rules apply.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct CompilationContext {
    pub release_destination: String,
    pub release_name: String,
    pub environment: String,
}

/// The labels-only projection of the topology graph exposed to the
/// mutator plugin.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct MutatorGraph {
    pub nodes: HashMap<String, MutatorNode>,
}

/// The per-node projection the mutator plugin evaluates compliance
/// rules against.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct MutatorNode {
    pub component: String,
    pub resolved_version: String,
    pub labels: HashMap<String, String>,
}

/// The response shape the mutator plugin emits: a list of opaque JSON
/// Patch documents the host applies to the topology graph. Patches
/// stay opaque at the type layer; plugin authors build them as
/// `serde_json::Value` values.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct MutatorOutput {
    pub patches: Vec<serde_json::Value>,
}

/// The JSON payload Step 4 hands to the policy-validator socket.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct PolicyValidatorInput {
    pub graph: PolicyGraph,
    pub precalculated_paths: Vec<PrecalculatedPath>,
}

/// The IR subset exposed to the policy validator.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct PolicyGraph {
    pub nodes: HashMap<String, PolicyNode>,
    pub edges: Vec<PolicyEdge>,
}

/// The per-node projection exposed to the policy validator. Narrower
/// than the full IR node surface: boundary path, subsystem path, sync
/// wave, and the annotation buckets are not exposed. `features` carries
/// the per-instance Feature attachment map; omitted from the JSON when
/// the node carries no attachments.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct PolicyNode {
    pub component: String,
    pub resolved_version: String,
    pub labels: HashMap<String, String>,
    #[serde(default, skip_serializing_if = "HashMap::is_empty")]
    pub features: HashMap<String, PolicyFeatureEntry>,
}

/// A Feature attachment exposed to the policy validator. `origin`
/// distinguishes attachments authored in a `features:` block
/// ("explicit") from synthetic attachments produced by the
/// annotation-driven expansion pass ("annotation").
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct PolicyFeatureEntry {
    #[serde(rename = "type")]
    pub r#type: String,
    pub resolved_version: String,
    #[serde(default, skip_serializing_if = "HashMap::is_empty")]
    pub values: HashMap<String, serde_json::Value>,
    #[serde(default, skip_serializing_if = "HashMap::is_empty")]
    pub infra: HashMap<String, serde_json::Value>,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub origin: String,
}

/// The per-edge projection exposed to the policy validator.
/// `origin_feature` distinguishes Component-authored edges (empty)
/// from Feature-authored edges (names the originating Feature
/// instance).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct PolicyEdge {
    pub source: String,
    pub target: String,
    pub socket: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub origin_feature: String,
}

/// One pre-enumerated routing path the analyzer materialises so policy
/// rules can reason about reachability without re-walking the graph.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct PrecalculatedPath {
    pub path_id: String,
    pub nodes: Vec<String>,
    pub attached_features: Vec<String>,
}

/// The response shape the policy-validator plugin emits.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct PolicyValidatorOutput {
    pub results: Vec<PolicyResult>,
}

/// One rule's verdict. `pass = false` halts the build with `message`
/// surfaced as a build-time diagnostic; `pass = true` is informational
/// and does not affect build outcome.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct PolicyResult {
    pub pass: bool,
    pub message: String,
}

// ----- Linter socket -----
//
// The linter socket carries the publishing-linter Wasm plugin's
// input + output. The PDK exposes the typed mirrors of the JSON
// wire shapes documented in
// `aggrega-schemas/json-schemas/wasm-linter-v1alpha1.schema.json`;
// per-language round-trip tests catch drift between this mirror and
// the schema.

/// The JSON payload the host hands the publishing-linter plugin on
/// each invocation. `rule_id` selects the rule the plugin dispatches
/// to (`"schema-evolution"` is the only recognised value at plugin
/// v0.1.0; subsequent rule WIs register more). `entity_kind`
/// identifies the entity kind the structured values encode.
/// `proposed` carries the parsed entity structure being classified
/// (YAML → JSON conversion happens host-side; the plugin consumes
/// the structured value via `serde_json`). `previous` is populated
/// for pairwise comparison rules (schema-evolution); `None` otherwise.
/// `policies` is the per-workspace publishing-discipline configuration
/// the rule consults.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct LinterInput {
    pub rule_id: String,
    pub entity_kind: String,
    pub proposed: serde_json::Value,
    /// Parsed structure of the previous-version entity, for rules
    /// that perform pairwise comparison (schema-evolution). `None`
    /// when the rule does not need a previous-version reference
    /// (first publication; structural-only rules).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub previous: Option<serde_json::Value>,
    /// Parsed structure of the resolved parent entity in the extends
    /// chain, for rules that perform child-vs-parent comparison
    /// (narrowing). The host caller resolves the entity's `spec.extends`
    /// URI before invocation. `None` when no parent exists or the rule
    /// does not need parent context.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub parent: Option<serde_json::Value>,
    /// Filesystem path the entity was loaded from. Populated by host
    /// callers walking the catalog filesystem; consumed by path-aware
    /// rules (e.g., category-matches-directory). `None` when the
    /// caller has no path context.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub source_path: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub policies: Option<Policies>,
}

/// The response shape the publishing-linter plugin emits. `findings`
/// carries one entry per rule invocation that produced a result.
/// The schema-evolution rule emits exactly one finding (the
/// classification verdict); structural rules added in later WIs may
/// emit zero or many findings.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct LinterOutput {
    pub findings: Vec<LinterFinding>,
}

/// One rule's verdict. `rule_id` names the rule that produced the
/// finding. `severity` distinguishes errors (publish-time halt) from
/// warnings (informational). `classification` carries the
/// PATCH/MINOR/MAJOR verdict from the schema-evolution rule; omitted
/// for rules that do not carry a classification. `message` is the
/// human-readable diagnostic surfaced in CLI output.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct LinterFinding {
    pub rule_id: String,
    pub severity: LinterSeverity,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub classification: Option<LinterClassification>,
    pub message: String,
}

/// PATCH / MINOR / MAJOR verdict from the schema-evolution rule.
/// Variants serialise to the upper-case wire names matching VS §2.13.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "UPPERCASE")]
pub enum LinterClassification {
    Patch,
    Minor,
    Major,
}

/// Diagnostic severity. `Error` halts the publish-time gate;
/// `Warning` is informational and does not affect the gate.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum LinterSeverity {
    Error,
    Warning,
}

/// Per-workspace publishing-discipline configuration the rule set
/// consults. Mirrors `workspaceconfig.PublishingDisciplineConfig`
/// in the Go host field-for-field; the schema's `Policies` $def
/// in `wasm-linter-v1alpha1.schema.json` is the cross-language
/// contract. A host-side round-trip test pins the symmetry.
#[derive(Debug, Clone, PartialEq, Default, Serialize, Deserialize)]
pub struct Policies {
    #[serde(default)]
    pub enforce_monotonic_narrowing: bool,
    #[serde(default)]
    pub enforce_immutability: bool,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub narrowing_exemptions: Vec<NarrowingExemption>,
    #[serde(default)]
    pub allow_dynamic_refs: bool,
    #[serde(default)]
    pub allow_id_overrides: bool,
    #[serde(default)]
    pub allow_direct_x_aggrega_extensions: bool,
    #[serde(default)]
    pub allow_aggrega_ai_prefix: bool,
    #[serde(default)]
    pub allow_nonstandard_names: bool,
    #[serde(default)]
    pub allow_category_mismatches: bool,
}

/// One per-entity exemption from the narrowing-violation rule.
/// `entity` names the catalog entity; `paths` enumerates the schema
/// paths that may legitimately widen; `reason` is the human-readable
/// justification surfaced to reviewers.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct NarrowingExemption {
    pub entity: String,
    pub paths: Vec<String>,
    pub reason: String,
}

