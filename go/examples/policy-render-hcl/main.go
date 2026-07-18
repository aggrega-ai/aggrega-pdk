// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Render-stage policy fixture that inspects a parsed HCL artifact tree.
//
// It declares the render stage, so the host runs it after render over the
// rendered artifacts. Where an artifact's format is one a policy guest does not
// read natively (HCL), the host parses it and carries the parsed JSON tree on the
// artifact. This fixture reads that tree, navigates its structure to enumerate
// the resource types the source declares under its top-level resource block, and
// emits a verdict driven by the attachment's settings.require_resource_type: it
// denies when the required resource type is absent from the parsed tree and
// records an audit verdict (letting the build proceed) when it is present.
// Driving it with a present versus an absent required type proves the guest reads
// the parsed HCL structure, not the raw source text.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-render-hcl-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"sort"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

type artifact struct {
	Path    string          `json:"path"`
	Content string          `json:"content"`
	Tree    json.RawMessage `json:"tree"`
}

type input struct {
	Artifacts []artifact `json:"artifacts"`
	Settings  struct {
		RequireResourceType string `json:"require_resource_type"`
	} `json:"settings"`
}

type result struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type output struct {
	Results []result `json:"results"`
}

// descriptor is the static describe() self-report: a v1alpha1 policy plugin
// evaluating the render stage.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "policy",
		Stage:       "render",
		Description: "Inspect the parsed HCL tree of the rendered artifacts and emit a verdict on the declared resource types.",
		Exports:     []aggrega.PluginExport{{Name: "validate"}},
		Metadata:    &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
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

// resourceTypes reads the parsed HCL tree and returns the sorted set of resource
// types declared under the top-level "resource" block. It navigates the tree
// structure — resource → <type> → <name> — so a result reflects the parsed
// structure rather than a substring of the raw source. Returns nil for an empty
// tree or one with no resource block.
func resourceTypes(tree json.RawMessage) []string {
	if len(tree) == 0 {
		return nil
	}
	var root map[string]any
	if err := json.Unmarshal(tree, &root); err != nil {
		return nil
	}
	resource, ok := root["resource"].(map[string]any)
	if !ok {
		return nil
	}
	types := make([]string, 0, len(resource))
	for t := range resource {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

//go:wasmexport validate
func validate() int32 {
	var in input
	_ = json.Unmarshal(pdk.Input(), &in)

	treeSeen := false
	var types []string
	for _, a := range in.Artifacts {
		if len(a.Tree) == 0 {
			continue
		}
		treeSeen = true
		types = append(types, resourceTypes(a.Tree)...)
	}
	sort.Strings(types)

	require := in.Settings.RequireResourceType
	present := false
	for _, t := range types {
		if t == require {
			present = true
		}
	}

	var out output
	switch {
	case !treeSeen:
		out.Results = []result{{Severity: "deny", Message: "render policy saw no parsed HCL tree on any artifact"}}
	case require != "" && !present:
		out.Results = []result{{Severity: "deny", Message: fmt.Sprintf("required resource type %q absent from parsed HCL tree; tree declares resource types %v", require, types)}}
	default:
		out.Results = []result{{Severity: "audit", Message: fmt.Sprintf("parsed HCL tree declares resource types %v; required resource type %q present", types, require)}}
	}

	b, err := json.Marshal(out)
	if err != nil {
		pdk.OutputString(`{"results":[{"severity":"deny","message":"fixture failed to encode output"}]}`)
		return 0
	}
	pdk.Output(b)
	return 0
}

func main() {}
