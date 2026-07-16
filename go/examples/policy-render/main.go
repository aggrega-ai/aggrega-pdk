// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Render-stage policy fixture plugin for the Aggrega Wasm runtime.
//
// It declares the render stage, so the host runs it after render over the
// rendered artifacts rather than over the IR graph. It reads its input payload:
// it echoes the artifact count and the first (path-sorted) artifact's path, and
// emits a single verdict whose severity is taken from the attachment's settings
// object (settings.severity), defaulting to audit when unset. Driving it with
// different settings exercises the render-stage deny / warn / audit host paths.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-render-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"

	pdk "github.com/extism/go-pdk"
)

type artifact struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type input struct {
	Artifacts []artifact `json:"artifacts"`
	Settings  struct {
		Severity string `json:"severity"`
	} `json:"settings"`
}

type result struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type output struct {
	Results []result `json:"results"`
}

// pluginDescriptor is the static describe() self-report: a v1alpha1
// policy plugin evaluating the render stage.
const pluginDescriptor = `{"abi_version":"v1alpha1","plugin_type":"policy","stage":"render","description":"Echo the rendered artifacts and emit a settings-driven verdict over the render output.","exports":[{"name":"validate"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}`

//go:wasmexport describe
func describe() int32 {
	pdk.Output([]byte(pluginDescriptor))
	return 0
}

//go:wasmexport validate
func validate() int32 {
	var in input
	_ = json.Unmarshal(pdk.Input(), &in)

	severity := in.Settings.Severity
	if severity == "" {
		severity = "audit"
	}

	msg := fmt.Sprintf("render stage saw %d artifact(s)", len(in.Artifacts))
	if len(in.Artifacts) > 0 {
		msg += fmt.Sprintf("; first artifact path=%q", in.Artifacts[0].Path)
	}
	msg += fmt.Sprintf("; settings.severity=%q", severity)

	out, err := json.Marshal(output{Results: []result{{Severity: severity, Message: msg}}})
	if err != nil {
		pdk.OutputString(`{"results":[{"severity":"deny","message":"fixture failed to encode output"}]}`)
		return 0
	}
	pdk.Output(out)
	return 0
}

func main() {}
