// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Reference feature-recipe-pipeline step plugin for the Aggrega Wasm
// runtime, authored with the Go PDK and built with the standard Go
// toolchain for GOOS=wasip1.
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
// It self-describes via describe() as a pipeline-step plugin whose step
// export declares operates=patch, scope=own, and the helm and helm-chart
// compatible providers. The host reads describe() once at load, validates
// the descriptor, then dispatches step with the keyed document set as
// input.
//
// The step payload is decoded and re-encoded through the generated ABI
// codec (github.com/aggrega-ai/aggrega-abi-go/pkg/abi) rather than
// hand-declared wire types. The socket carries canonical CBOR in both
// directions, so the step reads its input with DecodeCBOR and writes its
// output with EncodeCBOR. The codec is dispatch-free on the typed path (it
// decodes each nested type by a direct call to its own decoder and writes
// leaves by hand, with no reflective struct-walk), so the wire shape stays
// single-sourced from the schema.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o frp-step-go.wasm ./
package main

import (
	"encoding/json"
	"sort"

	abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"
	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// descriptor is the static describe() self-report: a v1alpha1 pipeline-step plugin
// whose step export patches own-scope workloads and is tested-compatible with
// the helm and helm-chart providers (both render workloads carrying a pod
// template, in flat-document and chart-source form respectively). The host
// reads this once at load and validates it before invoking step.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:   aggrega.APIVersion,
		PluginType:   "pipeline-step",
		Description:  "Patch a selected workload and emit companion documents in one feature-recipe pipeline step.",
		Capabilities: []string{"helm", "helm-chart"},
		Exports: []aggrega.PluginExport{{
			Name:     "step",
			Operates: "patch",
			Scope:    "own",
			Selector: "workloads carrying a pod template",
		}},
		Metadata: &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
}

// treeKind reports the kind field of a unit's tree, or "" when the unit
// is opaque (no tree) or carries no kind.
func treeKind(u abi.Unit) string {
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

//go:wasmexport describe
func describe() int32 {
	out, err := aggrega.MarshalDescriptor(descriptor())
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(out)
	return 0
}

//go:wasmexport step
func step() int32 {
	// Decode the real FRPStepInput through the generated codec's
	// DecodeCBOR — a streaming walk into the typed value, no reflective
	// struct-walk.
	var in abi.FRPStepInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(err)
		return 1
	}

	var out abi.FRPStepOutput

	// Content-select a Deployment in the own set and annotate it. The
	// guest learns the unit's key at runtime — it never knows emitted
	// filenames ahead of time — and roots its patch at that key. It reads
	// the unit's tree (skipping opaque units that carry none) and inspects
	// content. The own set arrives as an abi.OrderedMap, in the order the
	// host holds the documents; sort the keys so the patches come out in one
	// fixed order whatever that order is.
	ownKeys := in.Documents.Own.Keys()
	sort.Strings(ownKeys)
	for _, key := range ownKeys {
		unit, _ := in.Documents.Own.Get(key)
		if treeKind(unit) == "Deployment" {
			out.Patches = append(out.Patches, abi.JSONPatchOp{
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
	out.SideEffects = append(out.SideEffects, abi.SideEffect{
		Kind:  "emit_file",
		Scope: "own",
		File: &abi.SideEffectFile{
			Path:    "pipeline-configmap.yaml",
			Content: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: pipeline-emitted\ndata:\n  source: frp-step-go\n",
		},
	})

	// Host-scope EMIT: an additive marker the pipeline contributes to the
	// host component's set — only when a host set is present (a component-
	// scope FeatureRecipe attached to a host). A Recipe step has no host
	// scope, so the guest does not emit one. Additive emits run per-node
	// for every provider.
	if in.Documents.Host.Len() > 0 {
		out.SideEffects = append(out.SideEffects, abi.SideEffect{
			Kind:  "emit_file",
			Scope: "host",
			File: &abi.SideEffectFile{
				Path:    "pipeline-host-marker.yaml",
				Content: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: pipeline-host-marker\ndata:\n  injected-by: frp-step-go\n",
			},
		})
	}

	// Encode the output through the generated codec's EncodeCBOR — a
	// manual field-ordered build, no reflective struct-walk. The patch
	// Value is an any-typed JSON fragment the codec carries as the bytes of
	// its JSON document.
	raw, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(raw)
	return 0
}

func main() {}
