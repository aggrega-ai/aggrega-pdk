// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import (
	"encoding/json"
	"fmt"

	abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"
	pdk "github.com/extism/go-pdk"
)

// annotationDecision is the decision this role asks the engine for.
//
// The name is the adapter's own: the engine behind the seam is told which
// decision to return, and this is the word a policy author collects annotation
// decisions under. Change it and the policies written against this adapter change
// with it, so it is stated once, here.
const annotationDecision = "annotations"

// transform_annotations serves the annotation-transform role.
//
// What arrives: the components this attachment may annotate, each with the
// boundary it sits under, its own name and the annotations it already carries;
// plus the annotation keys the Manifest has registered a meaning for. An
// annotation under one of those keys goes on to expand into an attached feature,
// so a policy reads that list to inject keys that will take effect.
//
// What may be emitted: annotations to add to components that already exist. A
// decision carries a coordinate, a key and a string value, and there is no other
// field it could carry, so this role cannot express any other change. A decision
// naming a component the payload does not carry is refused by the build,
// naming the coordinate.
//
// A decision is read straight into the wire's own injection shape, so the field
// names a policy writes are the field names the build reads.
//
// How it travels: as for every role, the payload is read and the result written
// through the generated codec, and the decoded payload is handed to the seam as a
// JSON document.
//
//go:wasmexport transform_annotations
func transform_annotations() int32 {
	var in abi.AnnotationTransformInput
	if err := decode(&in, pdk.Input(), "annotation-transform"); err != nil {
		return fail(err)
	}
	payload, err := document(in, "annotation-transform")
	if err != nil {
		return fail(err)
	}

	decisions, err := Evaluate(
		programs(in.Sources, func(s abi.AnnotationSource) (string, string) { return s.Path, s.Content }),
		in.Settings,
		payload,
		annotationDecision,
	)
	if err != nil {
		return fail(fmt.Errorf("evaluating the annotation-transform policy: %w", err))
	}

	injections := make([]abi.AnnotationInjection, 0, len(decisions))
	for _, decision := range decisions {
		var injection abi.AnnotationInjection
		if err := json.Unmarshal(decision, &injection); err != nil {
			return fail(fmt.Errorf(
				"a decision collected under %q is not an annotation decision: %w. "+
					"A decision names %q, %q, %q and %q",
				annotationDecision, err,
				"boundary_path", "component_name", "annotation_key", "annotation_value",
			))
		}
		if injection.AnnotationKey == "" || injection.ComponentName == "" {
			return fail(fmt.Errorf(
				"a decision collected under %q leaves %q or %q empty; "+
					"an annotation with no key, or none to put it on, is not something a policy meant to ask for",
				annotationDecision, "annotation_key", "component_name",
			))
		}
		injections = append(injections, injection)
	}

	return emit(abi.AnnotationTransformOutput{Injections: injections}, "annotation-transform")
}
