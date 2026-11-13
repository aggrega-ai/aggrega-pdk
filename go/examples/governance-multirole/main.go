// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// End-to-end proof multi-role governance plugin for the Aggrega Wasm runtime.
//
// One governance guest that serves all four attachment roles of the unified
// governance socket, proving a single engine — not four distinct plugin types —
// can be attached at every role on one build. What it does at a site is fixed by
// the role the host attaches it at, not by anything the guest declares itself to
// be:
//
//   - annotation-transform: injects the first registered binding key onto the
//     first target (so it expands into a Feature attachment) plus one unbound key
//     (which rides through as a plain annotation);
//   - label-transform: stamps proof.aggrega/touched=mutator-proof on every node;
//   - ir-verdict / render-verdict: emits a single verdict whose severity is taken
//     from the attachment's settings (settings.severity), defaulting to info.
//
// Each verdict role has its own export, and both read only settings.severity,
// which the IR and render payloads both carry — so the two exports share one
// implementation once each has read its own payload, and the export the host
// dispatches is what says which stage the guest is running at.
//
// It self-describes via describe() as a governance plugin declaring all four
// governance exports; the host validates the export a role requires before it
// dispatches that role.
//
// Every role's payload and result travel as canonical CBOR, decoded and encoded
// by the generated codec on each role's shared payload types.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o governance-multirole-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	pdk "github.com/extism/go-pdk"

	abi "github.com/aggrega-ai/aggrega-abi/go/pkg/abi"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// proofLabelKey / proofLabelValue mirror the mutator-proof fixture: reusing the
// same label lets the empty-config no-op guard and the mutator->verdict chain key
// on one spelling. The key carries a "/" to exercise RFC 6901 escaping.
const (
	proofLabelKey   = "proof.aggrega/touched"
	proofLabelValue = "mutator-proof"
)

// injectedValue is the string the annotation facet injects for the bound
// annotation; it flows through binding expansion into the synthetic Feature's
// infra pillar and out into rendered output. unboundKey/unboundValue inject an
// annotation with no Manifest binding, to prove an unbound injection rides
// through as a plain annotation and produces no Feature (no silent compute).
const (
	injectedValue = "annotation-transform-was-here"
	unboundKey    = "proof.aggrega/unbound"
	unboundValue  = "no-binding-here"
)

// descriptor is the static describe() self-report: a governance plugin declaring
// all four governance exports. The host validates the export a role requires
// (transform_annotations / mutate / verdict_ir / verdict_render) before
// dispatching that role.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Serve every governance role from one engine: inject annotations, stamp labels, and emit settings-driven verdicts.",
		Exports: []aggrega.PluginExport{
			{Name: "transform_annotations"},
			{Name: "mutate"},
			{Name: "verdict_ir"},
			{Name: "verdict_render"},
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

// --- annotation-transform role ---

//go:wasmexport transform_annotations
func transform_annotations() int32 {
	var in abi.AnnotationTransformInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(fmt.Errorf("governance-multirole: decode AnnotationTransformInput: %w", err))
		return 1
	}
	if len(in.Targets) == 0 {
		return emitInjections(nil)
	}
	target := in.Targets[0]
	var injections []abi.AnnotationInjection
	if len(in.BindingKeys) > 0 {
		injections = append(injections, abi.AnnotationInjection{
			BoundaryPath:    target.BoundaryPath,
			ComponentName:   target.ComponentName,
			AnnotationKey:   in.BindingKeys[0],
			AnnotationValue: jsonString(injectedValue),
		})
	}
	injections = append(injections, abi.AnnotationInjection{
		BoundaryPath:    target.BoundaryPath,
		ComponentName:   target.ComponentName,
		AnnotationKey:   unboundKey,
		AnnotationValue: jsonString(unboundValue),
	})
	return emitInjections(injections)
}

// jsonString encodes a string annotation value as the JSON an injection carries
// — a string crosses as its quoted form, and an object of subkeys would cross as
// its own JSON. Encoding a string cannot fail.
func jsonString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func emitInjections(injections []abi.AnnotationInjection) int32 {
	b, err := abi.AnnotationTransformOutput{Injections: injections}.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("governance-multirole: encode AnnotationTransformOutput: %w", err))
		return 1
	}
	pdk.Output(b)
	return 0
}

// --- label-transform role ---

// escapePointerToken escapes a JSON Pointer reference token per RFC 6901: "~"
// becomes "~0" and "/" becomes "~1". Order matters — "~" is escaped first so a
// "/" rewritten to "~1" is not re-escaped.
func escapePointerToken(tok string) string {
	tok = strings.ReplaceAll(tok, "~", "~0")
	tok = strings.ReplaceAll(tok, "/", "~1")
	return tok
}

// mutate serves the label-transform role.
//
//go:wasmexport mutate
func mutate() int32 {
	var in aggrega.MutatorInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(fmt.Errorf("governance-multirole: decode MutatorInput: %w", err))
		return 1
	}
	// Sorted, so the patch list does not depend on the order the nodes arrive in.
	nodeIDs := in.Graph.Nodes.Keys()
	sort.Strings(nodeIDs)

	patches := make([]json.RawMessage, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		path := fmt.Sprintf("/nodes/%s/labels/%s",
			escapePointerToken(nodeID), escapePointerToken(proofLabelKey))
		patch := fmt.Sprintf(`[{"op":"add","path":%q,"value":%q}]`, path, proofLabelValue)
		patches = append(patches, json.RawMessage(patch))
	}
	b, err := aggrega.MutatorOutput{Patches: patches}.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("governance-multirole: encode MutatorOutput: %w", err))
		return 1
	}
	pdk.Output(b)
	return 0
}

// --- ir-verdict / render-verdict roles ---

// verdictSettings is the part of the attachment's settings both verdict roles
// read. The IR payload also carries the graph and precalculated paths, and the
// render payload the rendered artifacts; this proof reads neither, only the
// settings-driven severity, so once each export has read its own payload the two
// share one implementation and the role fixes which stage it runs at.
type verdictSettings struct {
	Severity string `json:"severity"`
}

// verdict_ir serves the ir-verdict role.
//
//go:wasmexport verdict_ir
func verdict_ir() int32 {
	var in aggrega.PolicyInput
	_ = in.DecodeCBOR(pdk.Input())
	return verdict(in.Settings)
}

// verdict_render serves the render-verdict role.
//
//go:wasmexport verdict_render
func verdict_render() int32 {
	var in abi.RenderPolicyInput
	_ = in.DecodeCBOR(pdk.Input())
	return verdict(in.Settings)
}

// verdict is the implementation both verdict exports share. It reads the one
// field both payloads carry, the attachment's settings, which arrive as the JSON
// document the attachment wrote. A payload it cannot read leaves the settings
// unread, and the verdict then reports the default severity — the proof reports
// what it was handed rather than refusing.
func verdict(rawSettings json.RawMessage) int32 {
	var settings verdictSettings
	if len(rawSettings) != 0 {
		_ = json.Unmarshal(rawSettings, &settings)
	}

	severity := settings.Severity
	if severity == "" {
		severity = aggrega.SeverityInfo
	}
	msg := fmt.Sprintf("governance-multirole verdict; settings.severity=%q", severity)

	b, err := aggrega.PolicyOutput{Results: []aggrega.Diagnostic{{Severity: severity, Message: msg}}}.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("governance-multirole: encode PolicyOutput: %w", err))
		return 1
	}
	pdk.Output(b)
	return 0
}

func main() {}
