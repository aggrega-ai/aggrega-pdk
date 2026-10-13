// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Adapter skeleton for a governance plugin: copy this directory, drop your
// policy engine into the seam, and the result is a guest an operator can attach
// at any governance role.
//
// This is a skeleton, not a framework. No policy engine is compiled in and no
// policy language is assumed. Everything specific to an engine lives behind one
// function, [Evaluate] in evaluator.go, which is the one file a new adapter has
// to rewrite. The role files around it — annotations.go, labels.go, verdicts.go —
// are the parts that stay the same whatever engine is behind the seam: each
// decodes the payload its role receives, hands it to the seam, and encodes the
// output its role is allowed to emit.
//
// # The four roles this guest serves
//
// A governance plugin is one guest attached at one of four roles, and the role
// an operator attaches it at — not anything the guest says about itself — fixes
// which payload arrives and what the guest may emit. All four are served here:
//
//	annotation-transform  transform_annotations  the components that may be annotated
//	label-transform       mutate                 the build's nodes and their labels
//	ir-verdict            verdict_ir             the build graph
//	render-verdict        verdict_render         the rendered output files
//
// Each role has its own export, so the export the host dispatches is what says
// which role the guest is serving and which payload it is reading. The two
// verdict exports share one implementation, because the two verdict payloads
// carry the same two members the seam needs.
//
// # The four things an adapter maps
//
// Whatever engine sits behind the seam, an adapter is wiring four things:
//
//   - sources become the engine's program text. The attachment's files arrive
//     verbatim; the host does not parse them.
//   - settings become the engine's data, and any translation the adapter itself
//     needs. The attachment's settings object arrives verbatim too.
//   - the input is the payload of the attached role, listed in the table above.
//   - the output is either the verdicts the engine reached, or the changes a
//     transform role asks for.
//
// # Build
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o governance-adapter-skeleton-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// descriptor is the self-report the host reads once when it loads the guest.
//
// It declares one export per governance role. The host refuses to call an export
// the guest did not declare, so an export is declared here in the same change
// that implements it — an attachment pointed at a missing export is then refused
// by name rather than silently doing nothing.
//
// An adapter serving fewer roles declares fewer exports and deletes the role
// files for the roles it does not serve. It declares no capabilities: a governance plugin is
// not selected by capability, and what it may emit comes from its role.
//
// SettingsSchema is left unset here, so the attachment's settings pass through
// verbatim. An adapter whose engine needs a settings shape can declare a JSON
// Schema document there; the host then validates the operator's settings against
// it before dispatch, and settings that do not conform halt the build.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Adapter skeleton: wire a policy engine to the governance roles.",
		Exports: []aggrega.PluginExport{
			{Name: "transform_annotations", Description: "Report the annotations the build's components should carry."},
			{Name: "mutate", Description: "Report the labels the build's nodes should carry."},
			{Name: "verdict_ir", Description: "Report the verdicts the policy reached over the build graph."},
			{Name: "verdict_render", Description: "Report the verdicts the policy reached over the rendered output."},
		},
		Metadata: &aggrega.PluginDescriptorMeta{License: "Apache-2.0"},
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

// fail reports a failure to the host and returns the non-zero status the export
// hands back.
//
// Every role in this skeleton refuses rather than returning an empty result when
// it cannot do its job. An adapter that swallowed a decode or evaluation failure
// and emitted no verdicts would let a build through on the strength of a policy
// that never ran, and the operator would have no way to see the difference.
//
// Failing the call is one of two channels the build reads as a failed invocation.
// The other is the error member every role's output carries: an adapter that can
// distinguish "evaluated, and here is what went wrong" from "could not run at
// all" may report the first there, alongside a partial result. This skeleton takes
// one channel rather than both, because two paths to one outcome is two places to
// keep in step. What neither channel permits is reporting a failure as an empty
// result, which on a governance surface reads as a policy that allowed the build.
func fail(err error) int32 {
	pdk.SetError(err)
	return 1
}

// programs normalizes one role's source entries into the seam's own view.
//
// Each role carries its own source type, all with the same two fields, so this
// is the single place the role payloads converge before the seam sees them. The
// caller supplies the accessor for its own type; the conversion itself has one
// home rather than one copy per role to keep in step.
func programs[S any](entries []S, read func(S) (path, text string)) []Program {
	out := make([]Program, 0, len(entries))
	for _, e := range entries {
		path, text := read(e)
		out = append(out, Program{Path: path, Text: text})
	}
	return out
}

// decode reads a role payload through the generated codec on the payload type
// itself, refusing anything it cannot read rather than proceeding on a zero
// value: a role that proceeded on a zero value would evaluate a policy against an
// empty document and report the result as a decision.
//
// Every role's wire carries the binary encoding, so this one reader serves all
// four. The parameter is the destination rather than a return value because the
// generated decode is a method on a pointer to the payload type.
func decode(in interface{ DecodeCBOR([]byte) error }, raw []byte, role string) error {
	if err := in.DecodeCBOR(raw); err != nil {
		return fmt.Errorf("reading the %s payload: %w", role, err)
	}
	return nil
}

// document writes a decoded role payload back out as the JSON document the seam
// reads, so the seam sees one document format at every role and a policy
// addresses the names the payload declares.
func document(in any, role string) (json.RawMessage, error) {
	b, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("reading the %s payload: %w", role, err)
	}
	return b, nil
}

// emit encodes a role's output through the generated codec on the output type
// itself, and hands it to the host.
func emit(out interface {
	EncodeCBOR() ([]byte, error)
}, role string) int32 {
	b, err := out.EncodeCBOR()
	if err != nil {
		return fail(fmt.Errorf("writing the %s result: %w", role, err))
	}
	pdk.Output(b)
	return 0
}

func main() {}
