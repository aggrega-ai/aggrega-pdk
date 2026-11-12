// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Observing fixture governance plugin for the ir-verdict role, for the Aggrega Wasm runtime.
//
// Unlike the constant-output policy fixtures, this plugin reads its input
// payload: it echoes the attachment-supplied sources (count, and the first
// source's base-name path and content length) and emits a single verdict whose
// severity is taken from the attachment's settings object (settings.severity),
// defaulting to info when unset. Driving the same plugin with different
// settings exercises the halt / warn / info host paths, and the echoed source
// detail proves the sources reached the guest as base names.
//
// The role's payload and result travel as canonical CBOR: the input is read with
// PolicyInput.DecodeCBOR and the output written with PolicyOutput.EncodeCBOR. The
// attachment's settings arrive as the JSON document the attachment wrote.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-observe-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"sort"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// settings is the part of the attachment's settings this fixture reads.
type settings struct {
	Severity string `json:"severity"`
}

// descriptor is the static describe() self-report: a governance plugin
// exporting verdict_ir, the ir-verdict role's default entry function.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Echo the attachment sources and emit a settings-driven verdict over the build graph.",
		Exports:     []aggrega.PluginExport{{Name: "verdict_ir"}},
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

//go:wasmexport verdict_ir
func verdict_ir() int32 {
	// The fixture reports what it was handed rather than refusing a payload it
	// reads only in part, so a read that stops early leaves the fields unread and
	// the verdict reports them as absent.
	var in aggrega.PolicyInput
	_ = in.DecodeCBOR(pdk.Input())
	var cfg settings
	if len(in.Settings) != 0 {
		_ = json.Unmarshal(in.Settings, &cfg)
	}

	severity := cfg.Severity
	if severity == "" {
		severity = aggrega.SeverityInfo
	}

	sources := in.Sources
	sort.Slice(sources, func(i, j int) bool { return sources[i].Path < sources[j].Path })

	msg := fmt.Sprintf("observed %d source(s)", len(sources))
	if len(sources) > 0 {
		msg += fmt.Sprintf("; first source path=%q content-length=%d", sources[0].Path, len(sources[0].Content))
	}
	msg += fmt.Sprintf("; settings.severity=%q", severity)

	out, err := aggrega.PolicyOutput{Results: []aggrega.Diagnostic{{Severity: severity, Message: msg}}}.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("policy-observe: encode PolicyOutput: %w", err))
		return 1
	}
	pdk.Output(out)
	return 0
}

func main() {}
