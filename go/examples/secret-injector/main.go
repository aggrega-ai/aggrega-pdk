// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Reference cross-entity injector step for the Aggrega feature-recipe pipeline,
// authored with the Go PDK and built with the standard Go toolchain for
// GOOS=wasip1.
//
// It is the downstream half of a producer-to-injector chain. It reads a secret
// name an earlier producer step supplied through the rendered `with` block —
// delivered as the step-local input the pipeline author wired from
// .steps.<id>.outputs.secretName — and PATCHes the consuming (host) workload to
// reference that secret: it content-selects the host chart's values.yaml and
// adds the secret name under the workload's environment block.
//
// It self-describes via describe() as a pipeline-step plugin whose step export
// declares operates=patch, scope=host, the helm-chart compatible provider (it
// patches chart source, so it is tested-compatible with the provider that emits
// chart source), and a typed input contract: a JSON Schema requiring a string
// secretName. The host reads describe() once at load and validates the rendered
// `with` input against that schema before the guest runs, so a missing or
// wrong-typed secretName halts the build.
//
// Selection is by content, never by filename: the guest cannot know the emitted
// key ahead of time, so it learns each unit's key at runtime and roots its
// /host PATCH at the values.yaml unit (a structured object carrying an image key
// and no apiVersion — Chart.yaml and rendered manifests carry apiVersion).
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o secret-injector-go.wasm ./
package main

import (
	"encoding/json"
	"errors"
	"sort"

	abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"
	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// inputContract is the JSON Schema the step export declares for its step-local
// input: an object carrying a string secretName. The host validates the
// rendered `with` input against it before the guest runs, so a missing or
// wrong-typed secretName halts the build before this guest sees it.
const inputContract = `{"type":"object","required":["secretName"],"properties":{"secretName":{"type":"string"}}}`

func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:   aggrega.APIVersion,
		PluginType:   "pipeline-step",
		Description:  "Patch the consuming workload to reference a secret named by an earlier producer step.",
		Capabilities: []string{"helm-chart"},
		Exports: []aggrega.PluginExport{{
			Name:     "step",
			Operates: "patch",
			Scope:    "host",
			Selector: "chart values.yaml of the attached host component",
			Input:    json.RawMessage(inputContract),
		}},
		Metadata: &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
}

// isChartValues reports whether a unit's tree is the host chart's values.yaml:
// an object carrying an image key and no apiVersion key. Chart.yaml carries
// apiVersion; rendered manifests under templates/ carry apiVersion; only
// values.yaml matches. Selection is by content, not by filename.
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

	// Read the secret name the earlier producer step supplied through the
	// rendered `with` block. The host validated it against the declared input
	// contract before this guest ran.
	rawName, ok := in.StepInput.Get("secretName")
	if !ok {
		pdk.SetError(errors.New("step input missing secretName"))
		return 1
	}
	var secretName string
	if err := json.Unmarshal(rawName, &secretName); err != nil {
		pdk.SetError(err)
		return 1
	}

	var out abi.FRPStepOutput

	// The host set arrives as an abi.OrderedMap, in the order the host holds
	// the documents; sort the keys so the patches come out in one fixed order
	// whatever that order is.
	hostKeys := in.Documents.Host.Keys()
	sort.Strings(hostKeys)
	for _, key := range hostKeys {
		unit, _ := in.Documents.Host.Get(key)
		if !isChartValues(unit) {
			continue
		}
		out.Patches = append(out.Patches, abi.JSONPatchOp{
			Scope: "host",
			Key:   key,
			Op:    "add",
			Path:  "/environment/INJECTED_SECRET_NAME",
			Value: secretName,
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
