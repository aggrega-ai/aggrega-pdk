// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"
	pdk "github.com/extism/go-pdk"
)

// labelDecision is the decision this role asks the engine for. See
// annotationDecision for why the name is the adapter's to choose and is stated
// once.
const labelDecision = "labels"

// The two operations a label decision may ask for.
const (
	labelOpAdd    = "add"
	labelOpRemove = "remove"
)

// labelChange is one label decision as a policy expresses it.
//
// A decision names the label it means — the node, the key and the value — rather
// than an edit instruction addressed by path. The reason is in pointer, below.
type labelChange struct {
	// Node is the name of the node the decision applies to.
	Node string `json:"node"`
	// Key is the label key the decision is about.
	Key string `json:"key"`
	// Value is the value the label takes. Required when adding, unused when
	// dropping.
	Value string `json:"value"`
	// Op is add or remove. Empty means add.
	Op string `json:"op"`
}

// instruction is one edit instruction, in the shape the build reads.
type instruction struct {
	Op    string  `json:"op"`
	Path  string  `json:"path"`
	Value *string `json:"value,omitempty"`
}

// mutate serves the label-transform role.
//
// What arrives: the build's nodes keyed by name, each carrying the component it
// instantiates, the version resolved for it and the labels it currently has;
// plus the Release's name and environment, so a policy can decide differently
// per environment.
//
// What may be emitted: label changes, one per decision. Every instruction this
// role builds addresses one label on one node, so a decision has no way to reach
// the node's component, its resolved version, the set of nodes, or a value that
// is not a string. That confinement is the whole of what an operator invited by
// attaching a plugin here.
//
// How it travels: the payload is read and the result written through the
// generated codec on the payload type. The seam reads a JSON document, so the
// decoded payload is written back out as one before it is handed over — the names
// a policy addresses are the payload's own names.
//
//go:wasmexport mutate
func mutate() int32 {
	var in abi.MutatorInput
	if err := decode(&in, pdk.Input(), "label-transform"); err != nil {
		return fail(err)
	}
	payload, err := document(in, "label-transform")
	if err != nil {
		return fail(err)
	}

	decisions, err := Evaluate(
		programs(in.Sources, func(s abi.MutatorSource) (string, string) { return s.Path, s.Content }),
		in.Settings,
		payload,
		labelDecision,
	)
	if err != nil {
		return fail(fmt.Errorf("evaluating the label-transform policy: %w", err))
	}

	patches := make([]json.RawMessage, 0, len(decisions))
	for _, decision := range decisions {
		var change labelChange
		if err := json.Unmarshal(decision, &change); err != nil {
			return fail(fmt.Errorf(
				"a decision collected under %q is not a label decision: %w. "+
					"A decision names %q, %q and %q, and may name %q as %q or %q",
				labelDecision, err, "node", "key", "value", "op", labelOpAdd, labelOpRemove,
			))
		}
		patch, err := labelPatch(change)
		if err != nil {
			return fail(err)
		}
		patches = append(patches, patch)
	}

	return emit(abi.MutatorOutput{Patches: patches}, "label-transform")
}

// labelPatch turns one decision into the single-operation edit document the
// build applies.
func labelPatch(change labelChange) (json.RawMessage, error) {
	op := change.Op
	if op == "" {
		op = labelOpAdd
	}
	var value *string
	switch op {
	case labelOpAdd:
		if change.Value == "" {
			return nil, fmt.Errorf(
				"the %q decision for node %q label %q names no %q; a label value is a string",
				labelDecision, change.Node, change.Key, "value",
			)
		}
		value = &change.Value
	case labelOpRemove:
	default:
		return nil, fmt.Errorf(
			"the %q decision for node %q label %q names %q %q, which is not one of %q or %q",
			labelDecision, change.Node, change.Key, "op", op, labelOpAdd, labelOpRemove,
		)
	}
	b, err := json.Marshal([]instruction{{
		Op:    op,
		Path:  "/nodes/" + pointer(change.Node) + "/labels/" + pointer(change.Key),
		Value: value,
	}})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// pointer escapes one segment of an edit instruction's path so a name containing
// the path separator addresses a single location rather than a nested one.
//
// This is why a policy names the label it means and this file builds the path.
// A label key routinely contains a "/" — it is the ordinary shape of a key, not
// an edge case — and "/" is the path separator, so a hand-written path would
// address a nested location and the change would be refused. Escaping it here
// writes the rule once instead of once per policy. The escape character is itself
// escaped first, so a separator rewritten into an escape sequence is not escaped
// twice.
func pointer(segment string) string {
	segment = strings.ReplaceAll(segment, "~", "~0")
	return strings.ReplaceAll(segment, "/", "~1")
}
