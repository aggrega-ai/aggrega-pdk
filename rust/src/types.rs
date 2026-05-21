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
