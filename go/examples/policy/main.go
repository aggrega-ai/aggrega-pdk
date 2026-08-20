// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Fixture policy plugin for the Aggrega Wasm runtime.
//
// It offers two policy entry points, so one guest covers both halves of the
// policy socket's output contract:
//
//   - validate emits a deterministic two-verdict PolicyOutput — one halt verdict
//     and one info verdict — reporting the rules that fired.
//   - report_incomplete emits a PolicyOutput carrying no verdicts and an error
//     message. That is the shape a policy plugin returns when it ran but could
//     not finish evaluating, and the host reads a present error as a failed
//     invocation rather than as a verdict the plugin reached.
//
// The fixture does not inspect the input graph; both outputs are constants so
// end-to-end tests can assert exact equality of the emitted shape.
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

// The fixture emits a constant PolicyOutput carrying an error and no verdicts.
// A policy plugin reaches for this shape when the evaluation itself did not
// complete — a rule source it could not read, an input it cannot interpret — as
// opposed to a halt verdict, which reports a rule that fired on a run that did
// complete. results stays present and empty because the wire shape requires it,
// and error carries the reason. Like fixtureOutput it is a hand-formatted
// literal, for the same reason: the fixture's wire shape is fixed, so a constant
// string is the simplest faithful encoding of what the wasm-policy JSON Schema
// describes.
const incompleteOutput = `{"results":[],` +
	`"error":"fixture rule set could not be evaluated: no rule source was supplied"}`

// descriptor is the static describe() self-report: a v1alpha1 governance plugin
// declaring both of the fixture's policy entry points. The host reads this once
// at load through the shared describe() gate, and refuses a call to any export
// the report does not declare — so an entry point has to be listed here to be
// dispatchable, whether it is the socket's default or one an attachment names.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Report deterministic halt and info policy verdicts over the build graph.",
		Exports: []aggrega.PluginExport{
			{Name: "validate"},
			{Name: "report_incomplete"},
		},
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

//go:wasmexport validate
func validate() int32 {
	pdk.OutputString(fixtureOutput)
	return 0
}

//go:wasmexport report_incomplete
func report_incomplete() int32 {
	pdk.OutputString(incompleteOutput)
	return 0
}

func main() {}
