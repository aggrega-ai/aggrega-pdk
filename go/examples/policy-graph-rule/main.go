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
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-graph-rule-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"sort"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// input is the narrow projection of the IR-stage verdict payload this guest
// reads: the graph's nodes (each with its labels) and the attachment settings.
// The full payload also carries edges, attachments, and precalculated paths; the
// decoder drops the fields this rule does not read.
type input struct {
	Graph struct {
		Spec struct {
			Nodes map[string]struct {
				Labels    map[string]string `json:"labels"`
				Component string            `json:"component"`
			} `json:"nodes"`
		} `json:"spec"`
	} `json:"graph"`
	Settings struct {
		RequiredLabel string `json:"required_label"`
		Severity      string `json:"severity"`
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
	var in input
	if err := json.Unmarshal(pdk.Input(), &in); err != nil {
		pdk.SetError(fmt.Errorf("policy-graph-rule: unmarshal PolicyInput: %w", err))
		return 1
	}

	required := in.Settings.RequiredLabel
	severity := in.Settings.Severity
	if severity == "" {
		severity = "halt"
	}

	// A rule with no required label to check is vacuously satisfied: emit no
	// verdict so the build proceeds.
	if required == "" {
		pdk.OutputString(`{"results":[]}`)
		return 0
	}

	// Walk nodes in sorted order so the first-offender the message names is
	// deterministic across runs of identical input.
	nodeIDs := make([]string, 0, len(in.Graph.Spec.Nodes))
	for id := range in.Graph.Spec.Nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)

	for _, id := range nodeIDs {
		if _, ok := in.Graph.Spec.Nodes[id].Labels[required]; !ok {
			msg := fmt.Sprintf("governance rule: node %q (component %q) lacks required label %q",
				id, in.Graph.Spec.Nodes[id].Component, required)
			emit(output{Results: []result{{Severity: severity, Message: msg}}})
			return 0
		}
	}

	// Every node carries the required label: no verdict, build proceeds.
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
