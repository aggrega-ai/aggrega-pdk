// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import (
	"encoding/json"
	"fmt"

	abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"
	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// The decisions the verdict roles ask the engine for.
//
// These three names are the adapter's own, and they are a convention rather than
// a contract: many policy languages already group decisions this way, so an
// adapter that adopts the same names lets a policy author reuse what they know.
// An adapter is free to pick other names, or to read them from the attachment's
// settings so an operator can choose.
//
// They are NOT the severities the wire carries. The wire's severities are the
// closed set below, and the mapping between the two is the table in
// verdictCollections. Keeping them distinct is what lets a policy author rename
// a decision collection without changing what the build does with it.
const (
	denyDecision  = "deny"
	warnDecision  = "warn"
	auditDecision = "audit"
)

// verdictCollections pairs each decision collection with the severity this
// adapter reports it as, in the order the results are emitted.
//
// The three severities are the whole closed set the wire carries: a halt stops
// the build and names what to fix, a warn surfaces alongside a build that still
// completes, and an info records the observation and leaves the build's outcome
// unchanged.
var verdictCollections = []struct {
	Decision string
	Severity string
}{
	{denyDecision, aggrega.SeverityHalt},
	{warnDecision, aggrega.SeverityWarn},
	{auditDecision, aggrega.SeverityInfo},
}

// validate serves both verdict roles.
//
// One export serves both because the payload identifies itself by shape: the
// render-verdict payload carries the rendered output files, the ir-verdict
// payload carries the build graph. Reading the shape is what lets this guest
// serve both without declaring which role it is attached at — and the role, not
// the guest, is what decides which payload arrives.
//
// What arrives at the ir-verdict role: the build graph, and the routing paths
// enumerated alongside it so a policy can reason about what reaches what without
// walking the graph itself.
//
// What arrives at the render-verdict role: the rendered output files, each with
// its output path, its rendered text, the format it was rendered in, and — where
// the host parsed the content before handing it over — a structured view of that
// content, so a policy can reason about a document it does not read natively. A
// file the host did not parse carries no structured view and is read as text.
//
// What may be emitted: verdicts, and nothing else. A verdict role reports an
// outcome; the build reads it as an outcome and never applies it as a change. An
// empty result means no verdict fired, which is a policy that allowed the build.
//
//go:wasmexport validate
func validate() int32 {
	raw := pdk.Input()

	// Which facet arrived is read from the payload rather than assumed, so a
	// payload this guest cannot read is refused instead of being evaluated as the
	// other facet and quietly finding nothing.
	facet, err := readVerdictFacet(raw)
	if err != nil {
		return fail(err)
	}

	var results []abi.Diagnostic
	for _, collection := range verdictCollections {
		decisions, err := Evaluate(facet.Programs, facet.Settings, raw, collection.Decision)
		if err != nil {
			return fail(fmt.Errorf("evaluating the %s policy: %w", facet.Role, err))
		}
		for _, decision := range decisions {
			var message string
			if err := json.Unmarshal(decision, &message); err != nil {
				return fail(fmt.Errorf(
					"a decision collected under %q is not a message: %w. "+
						"A verdict decision is the text the build reports",
					collection.Decision, err,
				))
			}
			results = append(results, abi.Diagnostic{Severity: collection.Severity, Message: message})
		}
	}
	if results == nil {
		results = []abi.Diagnostic{}
	}

	return emit(abi.PolicyOutput{Results: results}, facet.Role)
}

// verdictFacet is what the two verdict payloads have in common once the facet is
// known: the role that delivered it, its programs, and its settings.
type verdictFacet struct {
	Role     string
	Programs []Program
	Settings json.RawMessage
}

// readVerdictFacet reads the payload and reports which verdict role delivered it.
func readVerdictFacet(raw []byte) (verdictFacet, error) {
	var probe struct {
		Artifacts json.RawMessage `json:"artifacts"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return verdictFacet{}, fmt.Errorf("reading the verdict payload: %w", err)
	}

	read := func(s abi.PolicySource) (string, string) { return s.Path, s.Content }

	if probe.Artifacts != nil {
		in, err := decode[abi.RenderPolicyInput](raw, "render-verdict")
		if err != nil {
			return verdictFacet{}, err
		}
		return verdictFacet{
			Role:     "render-verdict",
			Programs: programs(in.Sources, read),
			Settings: in.Settings,
		}, nil
	}

	in, err := decode[abi.PolicyInput](raw, "ir-verdict")
	if err != nil {
		return verdictFacet{}, err
	}
	return verdictFacet{
		Role:     "ir-verdict",
		Programs: programs(in.Sources, read),
		Settings: in.Settings,
	}, nil
}
