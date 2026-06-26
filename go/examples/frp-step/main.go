// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Reference feature-recipe-pipeline step plugin for the Aggrega Wasm
// runtime, authored with the Go PDK and built with TinyGo
// (-target=wasm-unknown).
//
// Unlike a no-op fixture, this plugin exercises the real pipeline
// contract end-to-end:
//
//   - It content-selects a workload from the keyed document set (a
//     document whose kind is Deployment), learns that document's key at
//     runtime, and roots an own-scope PATCH at it — adding an annotation.
//   - It EMITs a new own-scope ConfigMap document (a pipeline side
//     effect).
//   - It EMITs a new host-scope marker document (an additive cross-entity
//     side effect) only when the host document set is present — a
//     component-scope FeatureRecipe attached to a host.
//
// It self-describes via describe() as an frp_step plugin whose step
// export declares operates=patch, scope=own, and the helm and helm-chart
// compatible providers. The host reads describe() once at load, validates
// the descriptor, then dispatches step with the keyed document set as
// input.
//
// Build: tinygo build -target=wasm-unknown -gc=conservative -o frp-step-go.wasm ./
// A non-leaking GC is required — the wasm-unknown default leaking collector
// breaks encoding/json's reflective decode of the keyed document set.
package main

import (
	"encoding/json"
	"sort"

	pdk "github.com/extism/go-pdk"
)

// pluginDescriptor is the static describe() self-report: a v1alpha1
// frp_step plugin whose step export patches own-scope workloads and is
// tested-compatible with the helm and helm-chart providers (both render
// workloads carrying a pod template, in flat-document and chart-source
// form respectively). The host reads this once at load and validates it
// before invoking step.
const pluginDescriptor = `{"abi_version":"v1alpha1","plugin_type":"frp_step","compatible_providers":["helm","helm-chart"],"exports":[{"name":"step","operates":"patch","scope":"own","selector":"workloads carrying a pod template"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}`

// unit is the subset of one keyed document the guest reads: the
// RFC-6902-addressable tree view. The token, bytes, and media type ride
// the wire too, but this plugin selects and patches by the tree alone. An
// opaque unit carries no tree and is skipped.
type unit struct {
	Tree json.RawMessage `json:"tree"`
}

// documentSet is the keyed, scope-rooted unit set the host hands the
// guest. The guest reads each unit's content to select, never a filename.
// host is present only for a component-scope FeatureRecipe attached to a
// host; the guest checks it before emitting a host-scope side effect.
type documentSet struct {
	Own  map[string]unit `json:"own"`
	Host map[string]unit `json:"host"`
}

// stepInput is the subset of the host's FRPStepInput this plugin reads.
// The step input, render context, and preceding steps ride on the wire
// but are unused here.
type stepInput struct {
	Documents documentSet `json:"documents"`
}

// jsonPatchOp is one RFC 6902 patch addressed to a document by scope and
// key.
type jsonPatchOp struct {
	Scope string `json:"scope"`
	Key   string `json:"key"`
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

// sideEffectFile is the file an emit_file side effect writes.
type sideEffectFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// sideEffect is one pipeline side effect (an additive EMIT).
type sideEffect struct {
	Kind  string         `json:"kind"`
	Scope string         `json:"scope,omitempty"`
	File  sideEffectFile `json:"file"`
}

// stepOutput is the host's FRPStepOutput wire shape: key-addressed
// patches plus additive side effects.
type stepOutput struct {
	Patches     []jsonPatchOp `json:"patches,omitempty"`
	SideEffects []sideEffect  `json:"side_effects,omitempty"`
}

// treeKind reports the kind field of a unit's tree, or "" when the unit
// is opaque (no tree) or carries no kind.
func treeKind(u unit) string {
	if u.Tree == nil {
		return ""
	}
	var doc struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(u.Tree, &doc); err != nil {
		return ""
	}
	return doc.Kind
}

//export describe
func describe() int32 {
	pdk.Output([]byte(pluginDescriptor))
	return 0
}

//export step
func step() int32 {
	// Decode the real FRPStepInput. The decode targets a value plus its
	// address (&in), never a nil typed pointer — json.Unmarshal into a
	// nil pointer fails.
	var in stepInput
	if err := json.Unmarshal(pdk.Input(), &in); err != nil {
		pdk.SetError(err)
		return 1
	}

	var out stepOutput

	// Content-select a Deployment in the own set and annotate it. The
	// guest learns the unit's key at runtime — it never knows emitted
	// filenames ahead of time — and roots its patch at that key. It reads
	// the unit's tree (skipping opaque units that carry none) and inspects
	// content. The own set arrives as a Go map, whose iteration order is
	// unspecified; sort the keys so selection is deterministic across runs.
	ownKeys := make([]string, 0, len(in.Documents.Own))
	for key := range in.Documents.Own {
		ownKeys = append(ownKeys, key)
	}
	sort.Strings(ownKeys)
	for _, key := range ownKeys {
		if treeKind(in.Documents.Own[key]) == "Deployment" {
			out.Patches = append(out.Patches, jsonPatchOp{
				Scope: "own",
				Key:   key,
				Op:    "add",
				// RFC 6901 escapes `/` in the annotation key as `~1`.
				Path:  "/metadata/annotations/aggrega.ai~1pipeline-patched",
				Value: "true",
			})
		}
	}

	// Own-scope EMIT: a new ConfigMap document the pipeline contributes.
	out.SideEffects = append(out.SideEffects, sideEffect{
		Kind:  "emit_file",
		Scope: "own",
		File: sideEffectFile{
			Path:    "pipeline-configmap.yaml",
			Content: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: pipeline-emitted\ndata:\n  source: frp-step-go\n",
		},
	})

	// Host-scope EMIT: an additive marker the pipeline contributes to the
	// host component's set — only when a host set is present (a component-
	// scope FeatureRecipe attached to a host). A Recipe step has no host
	// scope, so the guest does not emit one. Additive emits run per-node
	// for every provider.
	if len(in.Documents.Host) > 0 {
		out.SideEffects = append(out.SideEffects, sideEffect{
			Kind:  "emit_file",
			Scope: "host",
			File: sideEffectFile{
				Path:    "pipeline-host-marker.yaml",
				Content: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: pipeline-host-marker\ndata:\n  injected-by: frp-step-go\n",
			},
		})
	}

	raw, err := json.Marshal(out)
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(raw)
	return 0
}

func main() {}
