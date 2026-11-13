// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Reference producer step for the Aggrega feature-recipe pipeline, authored
// with the Go PDK and built with the standard Go toolchain for GOOS=wasip1.
//
// It stands at the head of a producer-to-injector chain. In one pipeline step
// it both:
//
//   - EMITs a secret-reference document into its own document set (a small
//     manifest that names a secret; the runtime environment's controller
//     resolves the reference at deploy time), and
//   - RETURNS an addressable output naming that secret, so a later injector
//     step reads the name through the pipeline's inter-step channel as
//     .steps.<id>.outputs.secretName and wires it into the consuming workload.
//
// It self-describes via describe() as a pipeline-step plugin whose step export
// declares operates=emit, scope=own, the helm compatible provider, and a typed
// output contract: a JSON Schema requiring a string secretName. The host reads
// describe() once at load and validates the guest's returned outputs against
// that schema after the step runs.
//
// The step payload is decoded and re-encoded through the generated ABI codec
// (github.com/aggrega-ai/aggrega-abi/go/pkg/abi) rather than hand-declared wire
// types, so the wire shape stays single-sourced from the schema. The socket
// carries canonical CBOR in both directions, so the step reads its input with
// DecodeCBOR and writes its output with EncodeCBOR.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o secret-ref-producer-go.wasm ./
package main

import (
	"encoding/json"

	abi "github.com/aggrega-ai/aggrega-abi/go/pkg/abi"
	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// managedSecretName is the deterministic name the producer writes into its
// emitted secret-reference document and returns as its secretName output. A
// fixed value keeps the guest's output byte-identical across runs of the same
// input.
const managedSecretName = "guardrail-managed-secret"

// outputContract is the JSON Schema the step export declares for its returned
// outputs: an object carrying a string secretName. The host validates the
// guest's returned outputs against it after the step runs, so an output that
// omits the field or gives it a non-string value halts the build.
const outputContract = `{"type":"object","required":["secretName"],"properties":{"secretName":{"type":"string"}}}`

// descriptor is the static describe() self-report: a pipeline-step plugin
// whose step export emits into its own set, declares the helm compatible
// provider, and types its returned outputs with the output contract above.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:   aggrega.APIVersion,
		PluginType:   "pipeline-step",
		Description:  "Emit a secret-reference document and return the secret's name for a later injector step to consume.",
		Capabilities: []string{"helm"},
		Exports: []aggrega.PluginExport{{
			Name:     "step",
			Operates: "emit",
			Scope:    "own",
			Selector: "the pipeline's own document set",
			Output:   json.RawMessage(outputContract),
		}},
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

//go:wasmexport step
func step() int32 {
	var in abi.FRPStepInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(err)
		return 1
	}

	var out abi.FRPStepOutput

	// Own-scope EMIT: a secret-reference document naming the managed secret.
	// The compile-time output is a deterministic pointer, not a fetched value —
	// the runtime environment's controller resolves the reference at deploy
	// time, so the build stays reproducible.
	out.SideEffects = append(out.SideEffects, abi.SideEffect{
		Kind:  "emit_file",
		Scope: "own",
		File: &abi.SideEffectFile{
			Path:    "secret-ref.yaml",
			Content: "apiVersion: aggrega.ai/v1beta1\nkind: SecretReference\nmetadata:\n  name: " + managedSecretName + "\nspec:\n  secretName: " + managedSecretName + "\n",
		},
	})

	// Return the secret's name as an addressable output. A later step reads it
	// through the pipeline's inter-step channel as .steps.<id>.outputs.secretName.
	name, err := json.Marshal(managedSecretName)
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	out.Outputs.Set("secretName", name)

	raw, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(raw)
	return 0
}

func main() {}
