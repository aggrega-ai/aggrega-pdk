// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Content-reading render-stage verdict fixture plugin for the Aggrega Wasm
// runtime.
//
// It attaches at the render-verdict role, so the host runs it after render over
// the rendered artifacts. It enforces a substantive content rule: no rendered
// artifact may contain a forbidden token, named by the attachment's
// settings.forbidden_token. It walks the artifacts (the host delivers them
// path-sorted) and, on the first artifact whose content contains the token, emits
// a verdict whose severity is taken from settings.severity (defaulting to halt).
// The message names the offending artifact and the token, so the halt proves the
// guest read the rendered content rather than emitting a constant. When no
// artifact contains the token the guest emits no verdict and the build proceeds.
//
// It self-describes via describe() as a governance plugin whose verdict_render
// export the host validates before dispatch. That export names the role it
// serves, so this guest runs at the render-verdict role over the artifacts.
//
// The role's payload and result travel as canonical CBOR: the input is read with
// RenderPolicyInput.DecodeCBOR and the output written with PolicyOutput.EncodeCBOR.
// The attachment's settings arrive as the JSON document the attachment wrote, so
// the rule's two settings are read out of it with encoding/json.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-content-rule-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"strings"

	pdk "github.com/extism/go-pdk"

	abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// settings is the part of the attachment's settings this rule reads.
type settings struct {
	ForbiddenToken string `json:"forbidden_token"`
	Severity       string `json:"severity"`
}

// descriptor is the static describe() self-report: a v1alpha1 governance plugin
// whose verdict_render export the host validates before dispatch.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Deny a build when a rendered artifact contains a forbidden token, reading the rule target from the rendered content.",
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

//go:wasmexport verdict_render
func verdict_render() int32 {
	var in abi.RenderPolicyInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(fmt.Errorf("policy-content-rule: decode RenderPolicyInput: %w", err))
		return 1
	}
	var cfg settings
	if len(in.Settings) != 0 {
		if err := json.Unmarshal(in.Settings, &cfg); err != nil {
			pdk.SetError(fmt.Errorf("policy-content-rule: read settings: %w", err))
			return 1
		}
	}

	token := cfg.ForbiddenToken
	severity := cfg.Severity
	if severity == "" {
		severity = aggrega.SeverityHalt
	}

	// A rule with no forbidden token is vacuously satisfied: no verdict, the
	// build proceeds.
	if token == "" {
		return emit(nil)
	}

	// The host delivers the artifacts path-sorted, so the first offender the
	// message names is deterministic across runs of identical input.
	for _, a := range in.Artifacts {
		if strings.Contains(a.Content, token) {
			msg := fmt.Sprintf("governance rule: artifact %q contains forbidden token %q", a.Path, token)
			return emit([]aggrega.Diagnostic{{Severity: severity, Message: msg}})
		}
	}

	// No artifact contains the token: no verdict, build proceeds.
	return emit(nil)
}

// emit encodes the verdicts reached as the PolicyOutput the host reads; an empty
// list is the form of "no verdict fired".
func emit(results []aggrega.Diagnostic) int32 {
	if results == nil {
		results = []aggrega.Diagnostic{}
	}
	b, err := aggrega.PolicyOutput{Results: results}.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("policy-content-rule: encode PolicyOutput: %w", err))
		return 1
	}
	pdk.Output(b)
	return 0
}

func main() {}
