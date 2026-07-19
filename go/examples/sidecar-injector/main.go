// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Reference cross-entity sidecar-injector step for the Aggrega feature-recipe
// pipeline, authored with the Go PDK and built with the standard Go toolchain
// for GOOS=wasip1.
//
// It PATCHes the consuming (host) workload to add a companion container: it
// content-selects the host chart's values.yaml and adds a named member under
// its containers map (an RFC 6902 add of a structured value). This is the
// additive-sidecar shape — a Feature that has to place a companion container
// alongside the consumer's own workload, which the host component did not
// declare itself.
//
// It self-describes via describe() as an frp_step plugin whose step export
// declares operates=patch, scope=host, and the helm-chart compatible provider
// (it patches chart source, so it is tested-compatible with the provider that
// emits chart source). It declares no typed input contract: the container it
// appends is fixed, so the step reads no step-local input.
//
// Selection is by content, never by filename: the guest learns each unit's key
// at runtime and roots its /host PATCH at the values.yaml unit (a structured
// object carrying an image key and no apiVersion).
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o sidecar-injector-go.wasm ./
package main

import (
	"encoding/json"
	"sort"

	abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"
	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:   aggrega.APIVersion,
		PluginType:   "frp_step",
		Description:  "Patch the consuming workload to add a companion sidecar container.",
		Capabilities: []string{"helm-chart"},
		Exports: []aggrega.PluginExport{{
			Name:     "step",
			Operates: "patch",
			Scope:    "host",
			Selector: "chart values.yaml of the attached host component",
		}},
		Metadata: &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
}

// isChartValues reports whether a unit's tree is the host chart's values.yaml:
// an object carrying an image key and no apiVersion key. Selection is by
// content, not by filename.
func isChartValues(u abi.Unit) bool {
	if u.Tree == nil {
		return false
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(u.Tree, &doc); err != nil {
		return false
	}
	_, hasImage := doc["image"]
	_, hasAPIVersion := doc["apiVersion"]
	return hasImage && !hasAPIVersion
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
	var in abi.FRPStepInput
	if err := in.UnmarshalJSON(pdk.Input()); err != nil {
		pdk.SetError(err)
		return 1
	}

	var out abi.FRPStepOutput

	// The host set arrives as a Go map, whose iteration order is unspecified;
	// sort the keys so selection is deterministic across runs. The added
	// container is a fixed map keyed by its own name under the containers map;
	// encoding/json emits its keys in sorted order, so the patch value is
	// byte-identical across runs.
	sidecar := map[string]string{"image": "guardrail-agent:latest"}
	hostKeys := make([]string, 0, len(in.Documents.Host))
	for key := range in.Documents.Host {
		hostKeys = append(hostKeys, key)
	}
	sort.Strings(hostKeys)
	for _, key := range hostKeys {
		if !isChartValues(in.Documents.Host[key]) {
			continue
		}
		out.Patches = append(out.Patches, abi.JSONPatchOp{
			Scope: "host",
			Key:   key,
			Op:    "add",
			Path:  "/containers/guardrail-sidecar",
			Value: sidecar,
		})
	}

	raw, err := out.MarshalJSON()
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(raw)
	return 0
}

func main() {}
