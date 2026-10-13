// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Negative-path producer step for the Aggrega feature-recipe pipeline, authored
// with the Go PDK for GOOS=wasip1. It exists to exercise the host's typed
// output-contract enforcement.
//
// Its step export declares a typed output contract requiring a string
// secretName, but the guest returns a number for that field. The host validates
// the guest's returned outputs against the declared schema after the step runs
// and halts the build on the violation. It is a deliberate contract-violating
// fixture, not a reference plugin — a well-behaved producer returns outputs that
// satisfy the schema it declares.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o secret-ref-producer-badoutput-go.wasm ./
package main

import (
	"encoding/json"

	abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"
	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// outputContract requires a string secretName; the guest deliberately returns a
// number for it, so the host's post-step output validation halts the build.
const outputContract = `{"type":"object","required":["secretName"],"properties":{"secretName":{"type":"string"}}}`

func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:   aggrega.APIVersion,
		PluginType:   "pipeline-step",
		Description:  "Return outputs that violate the declared output contract, to exercise output-contract enforcement.",
		Capabilities: []string{"helm"},
		Exports: []aggrega.PluginExport{{
			Name:     "step",
			Operates: "emit",
			Scope:    "own",
			Selector: "the pipeline's own document set",
			Output:   json.RawMessage(outputContract),
		}},
		Metadata: &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
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

//go:wasmexport step
func step() int32 {
	var in abi.FRPStepInput
	if err := in.UnmarshalJSON(pdk.Input()); err != nil {
		pdk.SetError(err)
		return 1
	}

	var out abi.FRPStepOutput
	// The declared output contract requires secretName to be a string; return a
	// number instead so the host's post-step output validation halts the build.
	out.Outputs.Set("secretName", json.RawMessage("12345"))

	raw, err := out.MarshalJSON()
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(raw)
	return 0
}

func main() {}
