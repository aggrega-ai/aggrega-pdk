// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Program is one policy program the attachment carried.
//
// It is the four roles' source entries normalized to one shape, so the engine
// behind the seam reads a single type rather than four that differ only in their
// declaring role.
type Program struct {
	// Path is the file's base name, as the attachment carried it. The host
	// supplies a base name and never a path from its own filesystem, so the same
	// attachment yields the same programs wherever it is built.
	Path string
	// Text is the program's source text, carried verbatim. Nothing outside this
	// guest has read it.
	Text string
}

// Evaluate is the seam. Replace its body with a call into the policy engine this
// adapter wraps; everything else in this skeleton can stay as it is.
//
// The four arguments are the four things an adapter maps:
//
//   - programs is the attachment's files, which are the engine's program text.
//   - settings is the attachment's settings object as raw bytes. It is the
//     engine's data, and the place to read any tunable the adapter itself needs.
//     It is nil when the attachment declares no settings.
//   - payload is the whole role payload as a JSON document, which is what the
//     engine evaluates against. Handing over the whole payload rather than a
//     projection of it means a policy reads the graph, the artifacts, the nodes
//     or the targets under the names the wire already gives them, so the input
//     is learnable from one place. A role whose wire carries the binary encoding
//     writes the decoded payload back out as JSON before handing it over, so the
//     seam sees one document format at every role and a policy addresses the same
//     names whichever encoding delivered them.
//   - decision names which decision the caller wants back. Each role asks for
//     its own; the names are the constants declared in the role files.
//
// Each returned element is one decision, as raw JSON. The role that asked for it
// reads it into the shape that role emits, so the field names a policy author
// writes are the field names the wire carries and there is no second spelling to
// keep in step.
//
// Two obligations on whatever replaces this body, both of which the roles around
// it rely on:
//
// Identical input must yield an identical sequence of decisions. A build's output
// is byte-identical across runs of identical input, and an adapter that iterated
// an unordered collection would break that from inside the guest. Where an engine
// yields a set, sort it before returning.
//
// A failure to evaluate must return an error, never an empty result. An empty
// result is a policy that found nothing to say; an error is a policy that did not
// run. Reporting the second as the first lets a build through unexamined.
//
// This placeholder body reaches no engine and returns no decisions, so a freshly
// copied skeleton emits each role's neutral result: no annotations, no labels,
// and one note per verdict role saying the seam is still empty. That is the
// wiring proof — the guest decoded what the host handed it and emitted the shape
// its role is allowed to emit.
func Evaluate(programs []Program, settings json.RawMessage, payload json.RawMessage, decision string) ([]json.RawMessage, error) {
	_ = payload

	// Everything below is placeholder, and goes when the engine arrives. It reaches
	// no engine and reads no program; it reports back what it was handed so that a
	// freshly built adapter says something at every role rather than being
	// indistinguishable from one that is silently doing nothing.
	if decision != auditDecision {
		return nil, nil
	}
	note, err := json.Marshal(fmt.Sprintf(
		"this adapter has no policy engine yet: the seam received %s and %s settings",
		received(programs), presence(settings),
	))
	if err != nil {
		return nil, err
	}
	return []json.RawMessage{note}, nil
}

// received lists the programs that reached the seam, for the placeholder note
// above. It goes with the placeholder.
func received(programs []Program) string {
	if len(programs) == 0 {
		return "no programs"
	}
	parts := make([]string, 0, len(programs))
	for _, p := range programs {
		parts = append(parts, fmt.Sprintf("%s (%d bytes)", p.Path, len(p.Text)))
	}
	return strings.Join(parts, ", ")
}

// presence describes whether the attachment carried settings, for the
// placeholder note above. It goes with the placeholder.
func presence(settings json.RawMessage) string {
	if len(settings) == 0 {
		return "no"
	}
	return "some"
}
