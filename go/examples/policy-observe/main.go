// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Observing policy fixture plugin for the Aggrega Wasm runtime.
//
// Unlike the constant-output policy fixtures, this plugin reads its input
// payload: it echoes the attachment-supplied sources (count, and the first
// source's base-name path and content length) and emits a single verdict whose
// severity is taken from the attachment's settings object (settings.severity),
// defaulting to audit when unset. Driving the same plugin with different
// settings exercises the deny / warn / audit host paths, and the echoed source
// detail proves the sources reached the guest as base names.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-observe-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"sort"

	pdk "github.com/extism/go-pdk"
)

// input is the narrow projection of the socket payload this fixture reads. The
// full payload also carries the IR graph and precalculated paths; this fixture
// ignores them (unknown/unused fields are dropped by the decoder) and reads only
// the attachment-supplied sources and settings.
type input struct {
	Sources  []source `json:"sources"`
	Settings struct {
		Severity string `json:"severity"`
	} `json:"settings"`
}

type source struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type result struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type output struct {
	Results []result `json:"results"`
}

// pluginDescriptor is the static describe() self-report: a v1alpha1
// policy plugin evaluating the ir stage.
const pluginDescriptor = `{"abi_version":"v1alpha1","plugin_type":"policy","stage":"ir","description":"Echo the attachment sources and emit a settings-driven verdict over the build graph.","exports":[{"name":"validate"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}`

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

	sort.Slice(in.Sources, func(i, j int) bool { return in.Sources[i].Path < in.Sources[j].Path })

	msg := fmt.Sprintf("observed %d source(s)", len(in.Sources))
	if len(in.Sources) > 0 {
		msg += fmt.Sprintf("; first source path=%q content-length=%d", in.Sources[0].Path, len(in.Sources[0].Content))
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
