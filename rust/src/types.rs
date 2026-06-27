// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Policy-validator socket payload types — hand-authored mirrors of the
//! JSON wire shape the Aggrega Policy Validator JSON Schema describes.
//!
//! The mutator and linter socket types are no longer hand-authored here:
//! they are generated from the canonical schemas into the portable
//! aggrega-abi crate and re-exported from `lib.rs`, single-sourcing the
//! wire shape. The policy types below remain hand-authored pending their
//! own schema-derived collapse.
//!
//! Each type derives `Serialize` + `Deserialize` so plugin authors can
//! round-trip them through `serde_json`. Field names match the JSON
//! Schema property names exactly.

use std::collections::HashMap;

use serde::{Deserialize, Serialize};

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

