// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package aggrega

import "encoding/json"

// MutatorInput is the JSON payload the host hands a mutator plugin on
// each invocation: release-level compilation context plus a labels-only
// projection of the topology graph. Field names mirror the JSON Schema
// at aggrega-schemas/json-schemas/wasm-mutator-v1alpha1.schema.json
// exactly.
type MutatorInput struct {
	CompilationContext CompilationContext `json:"compilation_context"`
	Graph              MutatorGraph       `json:"graph"`
}

// CompilationContext carries release-level metadata the mutator reads
// when deciding which compliance, FinOps, or governance rules apply.
type CompilationContext struct {
	ReleaseDestination string `json:"release_destination"`
	ReleaseName        string `json:"release_name"`
	Environment        string `json:"environment"`
}

// MutatorGraph is the labels-only projection of the topology graph
// exposed to the mutator plugin.
type MutatorGraph struct {
	Nodes map[string]MutatorNode `json:"nodes"`
}

// MutatorNode is the per-node projection the mutator plugin evaluates
// compliance rules against: which Component the node implements, which
// version resolved, and the node's label set.
type MutatorNode struct {
	Component       string            `json:"component"`
	ResolvedVersion string            `json:"resolved_version"`
	Labels          map[string]string `json:"labels"`
}

// MutatorOutput is the response shape the mutator plugin emits: a list
// of opaque JSON Patch documents the host applies to the topology
// graph. Patches stay opaque at the PDK type layer; plugin authors
// build them as json.RawMessage values.
type MutatorOutput struct {
	Patches []json.RawMessage `json:"patches"`
}

// PolicyValidatorInput is the JSON payload Step 4 hands to the
// policy-validator socket: a per-node policy projection of the IR
// graph plus the precalculated routing paths the analyzer enumerates
// alongside. Field names mirror the JSON Schema at
// aggrega-schemas/json-schemas/wasm-policy-validator-v1alpha1.schema.json.
type PolicyValidatorInput struct {
	Graph              PolicyGraph         `json:"graph"`
	PrecalculatedPaths []PrecalculatedPath `json:"precalculated_paths"`
}

// PolicyGraph is the IR subset exposed to the policy validator.
type PolicyGraph struct {
	Nodes map[string]PolicyNode `json:"nodes"`
	Edges []PolicyEdge          `json:"edges"`
}

// PolicyNode is the per-node projection exposed to the policy
// validator. Narrower than the full IR node surface: boundary path,
// subsystem path, sync wave, and the annotation buckets are not
// exposed. Features carries the per-instance Feature attachment map
// keyed by instance name and lets policies enforce required-feature
// or forbidden-feature rules; omitted from the JSON when the node
// carries no attachments.
type PolicyNode struct {
	Component       string                        `json:"component"`
	ResolvedVersion string                        `json:"resolved_version"`
	Labels          map[string]string             `json:"labels"`
	Features        map[string]PolicyFeatureEntry `json:"features,omitempty"`
}

// PolicyFeatureEntry is a Feature attachment exposed to the policy
// validator. Type and ResolvedVersion identify the catalog Feature and
// the resolver's version pin. Values and Infra carry the operational
// payloads; Origin distinguishes attachments authored in a features:
// block ("explicit") from synthetic attachments produced by the
// annotation-driven expansion pass ("annotation").
type PolicyFeatureEntry struct {
	Type            string         `json:"type"`
	ResolvedVersion string         `json:"resolved_version"`
	Values          map[string]any `json:"values,omitempty"`
	Infra           map[string]any `json:"infra,omitempty"`
	Origin          string         `json:"origin,omitempty"`
}

// PolicyEdge is the per-edge projection exposed to the policy
// validator. OriginFeature distinguishes Component-authored edges
// (empty) from Feature-authored edges (names the originating Feature
// instance).
type PolicyEdge struct {
	Source        string `json:"source"`
	Target        string `json:"target"`
	Socket        string `json:"socket"`
	OriginFeature string `json:"origin_feature,omitempty"`
}

// PrecalculatedPath is one pre-enumerated routing path the analyzer
// materialises so policy rules can reason about reachability without
// re-walking the graph.
type PrecalculatedPath struct {
	PathID           string   `json:"path_id"`
	Nodes            []string `json:"nodes"`
	AttachedFeatures []string `json:"attached_features"`
}

// PolicyValidatorOutput is the response shape the policy-validator
// plugin emits: one [PolicyResult] per evaluated rule. An empty
// Results slice signals "no rules ran" — distinct from "all rules
// passed", which is a slice of Pass=true results.
type PolicyValidatorOutput struct {
	Results []PolicyResult `json:"results"`
}

// PolicyResult is one rule's verdict. Pass=false halts the build with
// Message surfaced as a build-time diagnostic; Pass=true is
// informational and does not affect build outcome.
type PolicyResult struct {
	Pass    bool   `json:"pass"`
	Message string `json:"message"`
}
