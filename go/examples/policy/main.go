// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Fixture policy plugin for the Aggrega Wasm runtime.
//
// Emits a deterministic two-verdict PolicyOutput: one halt
// verdict and one info verdict. The fixture does not inspect the input
// graph; the outputs are constants so end-to-end tests can assert exact
// equality of the emitted shape.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-go.wasm ./
package main

import (
	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// The fixture emits a constant two-verdict PolicyOutput. The
// output is a hand-formatted literal — the fixture's wire shape is
// fixed, so a constant string is the simplest faithful encoding and
// matches the exact wire shape the wasm-policy JSON Schema
// describes. The PDK's own round-trip tests exercise the full
// encoding/json path against the same schema.
const fixtureOutput = `{"results":[` +
	`{"severity":"halt","message":"fixture rule B: deliberate fail for end-to-end coverage"},` +
	`{"severity":"info","message":"fixture rule A: advisory note"}` +
	`]}`

// descriptor is the static describe() self-report: a v1alpha1 policy plugin
// evaluating the ir stage, whose validate export the host validates before
// dispatch. The host reads this once at load through the shared describe() gate.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Report deterministic halt and info policy verdicts over the build graph.",
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

//go:wasmexport validate
func validate() int32 {
	pdk.OutputString(fixtureOutput)
	return 0
}

func main() {}
