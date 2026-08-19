// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Allow-all proof policy plugin for the Aggrega Wasm runtime.
//
// Emits a deterministic PolicyOutput with no fired verdicts, so
// the host-side end-to-end test can prove the policy socket lets a build
// succeed when no rule fires — the counterpart to the minimal
// policy fixture, whose halt verdict proves the build-halt path.
//
// It self-describes via describe() as a policy plugin whose
// validate export the host validates before dispatch.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-allow-go.wasm ./
package main

import (
	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// fixtureOutput is the constant no-verdict-fired output. The plugin does not
// inspect the input graph; the output is a fixed literal — an empty results
// array, the wire form of "no verdict fired" — so the end-to-end test asserts
// the build-succeeds path deterministically.
const fixtureOutput = `{"results":[]}`

// descriptor is the static describe() self-report: a v1alpha1 policy plugin
// evaluating the ir stage, whose validate export the host validates before
// dispatch.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion: aggrega.APIVersion,
		PluginType: "governance",
		Exports:    []aggrega.PluginExport{{Name: "validate"}},
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

//go:wasmexport validate
func validate() int32 {
	pdk.OutputString(fixtureOutput)
	return 0
}

func main() {}
