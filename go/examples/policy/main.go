// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Fixture governance plugin for the ir-verdict role, for the Aggrega Wasm runtime.
//
// It offers two entry functions, so one guest covers both halves of a verdict
// role's output contract:
//
//   - verdict_ir emits a deterministic two-verdict PolicyOutput — one halt
//     verdict and one info verdict — reporting the rules that fired.
//   - report_incomplete emits a PolicyOutput carrying no verdicts and an error
//     message. That is the shape a verdict-role plugin returns when it ran but could
//     not finish evaluating, and the host reads a present error as a failed
//     invocation rather than as a verdict the plugin reached.
//
// The fixture does not inspect the input graph; both outputs are constants so
// end-to-end tests can assert exact equality of the emitted shape. Every
// governance role's result travels as canonical CBOR, so each is written with
// PolicyOutput.EncodeCBOR, the generated codec on the shared payload type.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-go.wasm ./
package main

import (
	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// fixtureOutput is the constant two-verdict PolicyOutput the fixture emits: one
// halt verdict and one info verdict, in that order.
var fixtureOutput = aggrega.PolicyOutput{Results: []aggrega.Diagnostic{
	{Severity: aggrega.SeverityHalt, Message: "fixture rule B: deliberate fail for end-to-end coverage"},
	{Severity: aggrega.SeverityInfo, Message: "fixture rule A: advisory note"},
}}

// incompleteOutput is the constant PolicyOutput carrying an error and no
// verdicts. A verdict-role plugin reaches for this shape when the evaluation itself
// did not complete — a rule source it could not read, an input it cannot
// interpret — as opposed to a halt verdict, which reports a rule that fired on a
// run that did complete. results stays present and empty because the wire shape
// requires it, and error carries the reason.
var incompleteOutput = aggrega.PolicyOutput{
	Results: []aggrega.Diagnostic{},
	Error:   "fixture rule set could not be evaluated: no rule source was supplied",
}

// descriptor is the static describe() self-report: a governance plugin
// declaring both of the fixture's entry functions. The host reads this once
// at load through the shared describe() gate, and refuses a call to any export
// the report does not declare — so an entry point has to be listed here to be
// dispatchable, whether it is the socket's default or one an attachment names.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Report deterministic halt and info policy verdicts over the build graph.",
		Exports: []aggrega.PluginExport{
			{Name: "verdict_ir"},
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

//go:wasmexport verdict_ir
func verdict_ir() int32 {
	return emit(fixtureOutput)
}

//go:wasmexport report_incomplete
func report_incomplete() int32 {
	return emit(incompleteOutput)
}

// emit encodes a PolicyOutput and hands it back to the host.
func emit(out aggrega.PolicyOutput) int32 {
	b, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(b)
	return 0
}

func main() {}
