// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Fixture governance plugin for the render-verdict role that inspects a parsed HCL artifact tree.
//
// At the render-verdict role, the host runs it after render over the
// rendered artifacts. Where an artifact's format is one a policy guest does not
// read natively (HCL), the host parses it and carries the parsed JSON tree on the
// artifact. This fixture reads that tree, navigates its structure to enumerate
// the resource types the source declares under its top-level resource block, and
// emits a verdict driven by the attachment's settings.require_resource_type: a
// halt verdict when the required resource type is absent from the parsed tree and
// records an info verdict (letting the build proceed) when it is present.
// Driving it with a present versus an absent required type proves the guest reads
// the parsed HCL structure, not the raw source text.
//
// The role's payload and result travel as canonical CBOR: the input is read with
// RenderPolicyInput.DecodeCBOR and the output written with PolicyOutput.EncodeCBOR.
// An artifact's tree and the attachment's settings arrive as JSON documents, read
// with encoding/json.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-render-hcl-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"sort"

	pdk "github.com/extism/go-pdk"

	abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// settings is the part of the attachment's settings this fixture reads.
type settings struct {
	RequireResourceType string `json:"require_resource_type"`
}

// descriptor is the static describe() self-report: a governance plugin
// exporting verdict_render, the render-verdict role's default entry function.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Inspect the parsed HCL tree of the rendered artifacts and emit a verdict on the declared resource types.",
		Exports:     []aggrega.PluginExport{{Name: "verdict_render"}},
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

//go:wasmexport verdict_render
func verdict_render() int32 {
	// The fixture reports what it was handed rather than refusing a payload it
	// reads only in part: an unread artifact list reports that no tree was seen.
	var in abi.RenderPolicyInput
	_ = in.DecodeCBOR(pdk.Input())
	var cfg settings
	if len(in.Settings) != 0 {
		_ = json.Unmarshal(in.Settings, &cfg)
	}

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

	require := cfg.RequireResourceType
	present := false
	for _, t := range types {
		if t == require {
			present = true
		}
	}

	var out aggrega.PolicyOutput
	switch {
	case !treeSeen:
		out.Results = []aggrega.Diagnostic{{Severity: aggrega.SeverityHalt, Message: "render policy saw no parsed HCL tree on any artifact"}}
	case require != "" && !present:
		out.Results = []aggrega.Diagnostic{{Severity: aggrega.SeverityHalt, Message: fmt.Sprintf("required resource type %q absent from parsed HCL tree; tree declares resource types %v", require, types)}}
	default:
		out.Results = []aggrega.Diagnostic{{Severity: aggrega.SeverityInfo, Message: fmt.Sprintf("parsed HCL tree declares resource types %v; required resource type %q present", types, require)}}
	}

	b, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("policy-render-hcl: encode PolicyOutput: %w", err))
		return 1
	}
	pdk.Output(b)
	return 0
}

func main() {}
