// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// End-to-end proof annotation-injector plugin for the Aggrega Wasm runtime.
//
// It attaches at the annotation-transform governance role, which runs after
// resolve and before annotation expansion. It reads the resolved injection
// targets and the Manifest's registered binding keys, and injects two
// annotations onto the first target: the first registered binding key (so the
// injection expands into a Feature attachment), and a fixed unregistered key (so
// the host-and-pipeline treat it exactly as an authored unbound annotation — no
// silent compute). The host applies only annotation additions to the coordinates
// it supplied as targets, and stamps its own aggrega.ai/injected-by provenance
// marker; the guest forges no provenance.
//
// It self-describes via describe() as a governance plugin whose
// transform_annotations export the host validates before dispatch.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o annotation-injector-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// injectedValue is the value the plugin injects for the bound annotation. The
// target Feature's schema types the routed field as a string, so the value is a
// string; it flows through binding expansion into the synthetic Feature's infra
// pillar and out into the component's rendered output.
const injectedValue = "annotation-transform-was-here"

// unboundKey is a fixed annotation key with no Manifest binding. The plugin
// injects it to prove an unbound injected annotation behaves exactly as an
// authored unbound annotation: it rides through as a plain annotation and
// produces no Feature attachment (no silent compute).
const unboundKey = "proof.aggrega/unbound"

// unboundValue is the value the plugin injects for the unbound annotation.
const unboundValue = "no-binding-here"

// descriptor is the static describe() self-report: a v1alpha1 governance plugin
// whose transform_annotations export the host validates before dispatch.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion: aggrega.APIVersion,
		PluginType: "governance",
		Exports:    []aggrega.PluginExport{{Name: "transform_annotations"}},
		Metadata:   &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
}

// annotationTransformInput is the minimal shape the plugin reads from the
// AnnotationTransformInput payload: the target coordinates it may inject onto and
// the registered binding keys. The full payload carries more (per-target current
// annotations, sources, settings); this proof plugin ignores those.
type annotationTransformInput struct {
	Targets []struct {
		BoundaryPath  string `json:"boundary_path"`
		ComponentName string `json:"component_name"`
	} `json:"targets"`
	BindingKeys []string `json:"binding_keys"`
}

// annotationInjection mirrors the host's AnnotationInjection wire shape.
type annotationInjection struct {
	BoundaryPath    string `json:"boundary_path"`
	ComponentName   string `json:"component_name"`
	AnnotationKey   string `json:"annotation_key"`
	AnnotationValue string `json:"annotation_value"`
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

//go:wasmexport transform_annotations
func transform_annotations() int32 {
	var in annotationTransformInput
	if err := json.Unmarshal(pdk.Input(), &in); err != nil {
		pdk.SetError(fmt.Errorf("annotation-injector: unmarshal AnnotationTransformInput: %w", err))
		return 1
	}

	// Nothing to inject onto: emit an empty injection list. The host applies
	// nothing and the build is byte-identical to one without the plugin.
	if len(in.Targets) == 0 {
		emit(nil)
		return 0
	}

	target := in.Targets[0]
	var injections []annotationInjection

	// Bound: inject the first registered binding key so it expands into a
	// Feature attachment. The host has ordered the targets and keys
	// deterministically, so the same input yields the same injection.
	if len(in.BindingKeys) > 0 {
		injections = append(injections, annotationInjection{
			BoundaryPath:    target.BoundaryPath,
			ComponentName:   target.ComponentName,
			AnnotationKey:   in.BindingKeys[0],
			AnnotationValue: injectedValue,
		})
	}

	// Unbound: inject a key with no binding. It rides through as a plain
	// annotation and produces no Feature attachment.
	injections = append(injections, annotationInjection{
		BoundaryPath:    target.BoundaryPath,
		ComponentName:   target.ComponentName,
		AnnotationKey:   unboundKey,
		AnnotationValue: unboundValue,
	})

	emit(injections)
	return 0
}

// emit marshals the injection list into the AnnotationTransformOutput wire shape
// and hands it back to the host.
func emit(injections []annotationInjection) {
	out := struct {
		Injections []annotationInjection `json:"injections"`
	}{Injections: injections}
	outputBytes, err := json.Marshal(out)
	if err != nil {
		pdk.SetError(fmt.Errorf("annotation-injector: marshal AnnotationTransformOutput: %w", err))
		return
	}
	pdk.Output(outputBytes)
}

func main() {}
