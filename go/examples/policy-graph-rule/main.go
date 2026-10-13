// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Graph-reading verdict fixture plugin for the Aggrega Wasm runtime.
//
// It attaches at a verdict role and reasons over the IR graph the host hands it:
// it enforces a substantive declaration rule — every node must carry a required
// governance label, named by the attachment's settings.required_label. It walks
// the graph's nodes in sorted order and, on the first node that lacks the
// required label, emits a verdict whose severity is taken from settings.severity
// (defaulting to halt). The verdict message names the offending node and the
// missing label, so the halt proves the guest read the graph rather than emitting
// a constant. When every node carries the label the guest emits no verdict, so
// the build proceeds.
//
// Because the rule signal is a specific node label, a label-transform guest that
// stamps that label upstream flips this verdict — the mechanism a mutator->verdict
// chain proof exercises.
//
// It self-describes via describe() as a governance plugin whose verdict_ir
// export the host validates before dispatch. That export names the role it
// serves, so this guest runs at the ir-verdict role over the build graph.
//
// The role's payload and result travel as canonical CBOR: the input is read with
// PolicyInput.DecodeCBOR and the output written with PolicyOutput.EncodeCBOR. The
// attachment's settings arrive as the JSON document the attachment wrote, so the
// rule's two settings are read out of it with encoding/json.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-graph-rule-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"sort"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// settings is the part of the attachment's settings this rule reads.
type settings struct {
	RequiredLabel string `json:"required_label"`
	Severity      string `json:"severity"`
}

// descriptor is the static describe() self-report: a v1alpha1 governance plugin
// whose verdict_ir export the host validates before dispatch.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Deny a build when an IR node lacks a required governance label, reading the rule target from the graph.",
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
	var in aggrega.PolicyInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(fmt.Errorf("policy-graph-rule: decode PolicyInput: %w", err))
		return 1
	}
	var cfg settings
	if len(in.Settings) != 0 {
		if err := json.Unmarshal(in.Settings, &cfg); err != nil {
			pdk.SetError(fmt.Errorf("policy-graph-rule: read settings: %w", err))
			return 1
		}
	}

	required := cfg.RequiredLabel
	severity := cfg.Severity
	if severity == "" {
		severity = aggrega.SeverityHalt
	}

	// A rule with no required label to check is vacuously satisfied: emit no
	// verdict so the build proceeds.
	if required == "" {
		return emit(nil)
	}

	// Walk nodes in sorted order so the first-offender the message names is
	// deterministic across runs of identical input.
	nodeIDs := in.Graph.Spec.Nodes.Keys()
	sort.Strings(nodeIDs)

	for _, id := range nodeIDs {
		node, _ := in.Graph.Spec.Nodes.Get(id)
		if _, ok := node.Labels.Get(required); !ok {
			msg := fmt.Sprintf("governance rule: node %q (component %q) lacks required label %q",
				id, node.Component, required)
			return emit([]aggrega.Diagnostic{{Severity: severity, Message: msg}})
		}
	}

	// Every node carries the required label: no verdict, build proceeds.
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
		pdk.SetError(fmt.Errorf("policy-graph-rule: encode PolicyOutput: %w", err))
		return 1
	}
	pdk.Output(b)
	return 0
}

func main() {}
