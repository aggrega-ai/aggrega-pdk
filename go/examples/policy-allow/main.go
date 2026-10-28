// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Allow-all proof governance plugin for the ir-verdict role, for the Aggrega Wasm runtime.
//
// Emits a deterministic PolicyOutput with no fired verdicts, so
// the host-side end-to-end test can prove an ir-verdict attachment lets a build
// succeed when no rule fires — the counterpart to the minimal
// policy fixture, whose halt verdict proves the build-halt path.
//
// It self-describes via describe() as a governance plugin whose
// verdict_ir export the host validates before dispatch. Its result travels as
// canonical CBOR, written with PolicyOutput.EncodeCBOR.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-allow-go.wasm ./
package main

import (
	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// fixtureOutput is the constant no-verdict-fired output. The plugin does not
// inspect the input graph; the output is fixed — an empty results list, the form
// of "no verdict fired" — so the end-to-end test asserts the build-succeeds path
// deterministically.
var fixtureOutput = aggrega.PolicyOutput{Results: []aggrega.Diagnostic{}}

// descriptor is the static describe() self-report: a v1alpha1 governance plugin
// for the ir-verdict role, whose verdict_ir export the host validates before
// dispatch.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion: aggrega.APIVersion,
		PluginType: "governance",
		Exports:    []aggrega.PluginExport{{Name: "verdict_ir"}},
		Metadata:   &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
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

//go:wasmexport verdict_ir
func verdict_ir() int32 {
	out, err := fixtureOutput.EncodeCBOR()
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(out)
	return 0
}

func main() {}
