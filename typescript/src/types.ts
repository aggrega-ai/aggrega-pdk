// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Policy-validator socket payload types — hand-authored mirrors of the
 * JSON wire shape the Aggrega Policy Validator JSON Schema describes.
 *
 * The mutator and linter socket types are generated from the canonical
 * schemas into the portable @aggrega-ai/aggrega-abi package and
 * re-exported from index.ts, single-sourcing the wire shape. The policy
 * types below remain hand-authored pending their own schema-derived
 * collapse. Field names match the JSON Schema property names exactly.
 */

export interface PolicyFeatureEntry {
  type: string;
  resolved_version: string;
  values?: Record<string, unknown>;
  infra?: Record<string, unknown>;
  origin?: string;
}

export interface PolicyNode {
  component: string;
  resolved_version: string;
  labels: Record<string, string>;
  features?: Record<string, PolicyFeatureEntry>;
}

export interface PolicyEdge {
  source: string;
  target: string;
  socket: string;
  origin_feature?: string;
}

export interface PrecalculatedPath {
  path_id: string;
  nodes: string[];
  attached_features: string[];
}

export interface PolicyGraph {
  nodes: Record<string, PolicyNode>;
  edges: PolicyEdge[];
}

export interface PolicyValidatorInput {
  graph: PolicyGraph;
  precalculated_paths: PrecalculatedPath[];
}

export interface PolicyResult {
  pass: boolean;
  message: string;
}

export interface PolicyValidatorOutput {
  results: PolicyResult[];
}
