// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Render-stage policy fixture plugin for the Aggrega Wasm runtime.
//
// It declares the render stage, so the host runs it after render over the
// rendered artifacts rather than over the IR graph. It reads its input payload:
// it echoes the artifact count and the first (path-sorted) artifact's path, and
// emits a single verdict whose severity is taken from the attachment's settings
// object (settings.severity), defaulting to info when unset. Driving it with
// different settings exercises the render-stage deny / warn / audit host paths.
//
// The role's payload and result travel as canonical CBOR: the input is read with
// RenderPolicyInput.DecodeCBOR and the output written with PolicyOutput.EncodeCBOR.
// The attachment's settings arrive as the JSON document the attachment wrote.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-render-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"

	pdk "github.com/extism/go-pdk"

	abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// settings is the part of the attachment's settings this fixture reads.
type settings struct {
	Severity string `json:"severity"`
}

// descriptor is the static describe() self-report: a v1alpha1 policy plugin
// evaluating the render stage.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Echo the rendered artifacts and emit a settings-driven verdict over the render output.",
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
	// The fixture reports what it was handed rather than refusing a payload it
	// reads only in part, so a read that stops early leaves the fields unread and
	// the verdict reports them as absent.
	var in abi.RenderPolicyInput
	_ = in.DecodeCBOR(pdk.Input())
	var cfg settings
	if len(in.Settings) != 0 {
		_ = json.Unmarshal(in.Settings, &cfg)
	}

	severity := cfg.Severity
	if severity == "" {
		severity = aggrega.SeverityInfo
	}

	msg := fmt.Sprintf("render stage saw %d artifact(s)", len(in.Artifacts))
	if len(in.Artifacts) > 0 {
		msg += fmt.Sprintf("; first artifact path=%q", in.Artifacts[0].Path)
	}
	msg += fmt.Sprintf("; settings.severity=%q", severity)

	out, err := aggrega.PolicyOutput{Results: []aggrega.Diagnostic{{Severity: severity, Message: msg}}}.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("policy-render: encode PolicyOutput: %w", err))
		return 1
	}
	pdk.Output(out)
	return 0
}

func main() {}
