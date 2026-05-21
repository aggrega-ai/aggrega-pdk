// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Socket payload types — hand-authored mirrors of the JSON wire shape
 * the Aggrega Wasm Mutator and Policy Validator JSON Schemas describe.
 *
 * The types are independent re-implementations of the wire shape that
 * the Go and Rust PDKs also mirror. Field names match the JSON Schema
 * property names exactly (snake_case wire format). Drift between any
 * mirror and the JSON Schema is caught by the round-trip mirroring
 * tests in `tests/`.
 */

export interface CompilationContext {
  release_destination: string;
  release_name: string;
  environment: string;
}

export interface MutatorNode {
  component: string;
  resolved_version: string;
  labels: Record<string, string>;
}

export interface MutatorGraph {
  nodes: Record<string, MutatorNode>;
}

export interface MutatorInput {
  compilation_context: CompilationContext;
  graph: MutatorGraph;
}

export interface MutatorOutput {
  patches: unknown[];
}

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
