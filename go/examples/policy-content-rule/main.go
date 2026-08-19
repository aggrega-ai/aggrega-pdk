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
// It self-describes via describe() as a governance plugin whose validate export
// the host validates before dispatch; the render-verdict role fixes that it runs
// after render over the artifacts.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-content-rule-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"strings"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// input is the narrow projection of the render-stage verdict payload this guest
// reads: the rendered artifacts (path and content) and the attachment settings.
type input struct {
	Artifacts []struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	} `json:"artifacts"`
	Settings struct {
		ForbiddenToken string `json:"forbidden_token"`
		Severity       string `json:"severity"`
	} `json:"settings"`
}

type result struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type output struct {
	Results []result `json:"results"`
}

// descriptor is the static describe() self-report: a v1alpha1 governance plugin
// whose validate export the host validates before dispatch.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Deny a build when a rendered artifact contains a forbidden token, reading the rule target from the rendered content.",
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
	var in input
	if err := json.Unmarshal(pdk.Input(), &in); err != nil {
		pdk.SetError(fmt.Errorf("policy-content-rule: unmarshal RenderPolicyInput: %w", err))
		return 1
	}

	token := in.Settings.ForbiddenToken
	severity := in.Settings.Severity
	if severity == "" {
		severity = "halt"
	}

	// A rule with no forbidden token is vacuously satisfied: no verdict, the
	// build proceeds.
	if token == "" {
		pdk.OutputString(`{"results":[]}`)
		return 0
	}

	// The host delivers the artifacts path-sorted, so the first offender the
	// message names is deterministic across runs of identical input.
	for _, a := range in.Artifacts {
		if strings.Contains(a.Content, token) {
			msg := fmt.Sprintf("governance rule: artifact %q contains forbidden token %q", a.Path, token)
			emit(output{Results: []result{{Severity: severity, Message: msg}}})
			return 0
		}
	}

	// No artifact contains the token: no verdict, build proceeds.
	pdk.OutputString(`{"results":[]}`)
	return 0
}

func emit(o output) {
	b, err := json.Marshal(o)
	if err != nil {
		pdk.OutputString(`{"results":[{"severity":"halt","message":"fixture failed to encode output"}]}`)
		return
	}
	pdk.Output(b)
}

func main() {}
