// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Fixture mutator plugin for the Aggrega Wasm runtime.
//
// Applies one trivial transform — appends the label
// "fixture.aggrega-pdk/touched=mutator-go" to every node in the
// MutatorGraph — and returns one JSON Patch document per node carrying
// that addition. The host can verify end-to-end that:
//
//   - The plugin loaded successfully via the Aggrega Wasm loader.
//   - Input JSON deserialised to MutatorInput.
//   - The transform applied per node.
//   - Output JSON serialises to a MutatorOutput valid against the
//     wasm-mutator JSON Schema.
//
// Build: tinygo build -target=wasm-unknown -o mutator-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"sort"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// fixtureLabelKey is the label key the fixture appends to every node.
// Choice of "fixture.aggrega-pdk/touched" identifies the source of the
// label as the fixture plugin so end-to-end tests on the host can
// recognise the transform.
const fixtureLabelKey = "fixture.aggrega-pdk/touched"

// fixtureLabelValue is the label value the fixture writes. The fixture
// is the Go-language fixture; the value identifies the source language
// so cross-language fixture-parity tests can distinguish per-language
// outputs when multiple PDKs ship.
const fixtureLabelValue = "mutator-go"

//export mutate
func mutate() int32 {
	inputBytes := pdk.Input()

	var in aggrega.MutatorInput
	if err := json.Unmarshal(inputBytes, &in); err != nil {
		pdk.SetError(fmt.Errorf("mutator-go: unmarshal MutatorInput: %w", err))
		return 1
	}

	// Iterate node IDs in sorted order. Go map iteration is
	// non-deterministic; sorted iteration keeps the emitted patches
	// byte-identical across runs of identical input — both to match
	// Aggrega's determinism discipline and to keep this fixture safe
	// to copy as a starting template for real mutator plugins.
	nodeIDs := make([]string, 0, len(in.Graph.Nodes))
	for nodeID := range in.Graph.Nodes {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)

	patches := make([]json.RawMessage, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		patch := fmt.Sprintf(`[{"op":"add","path":"/nodes/%s/labels/%s","value":%q}]`,
			nodeID, fixtureLabelKey, fixtureLabelValue)
		patches = append(patches, json.RawMessage(patch))
	}

	out := aggrega.MutatorOutput{Patches: patches}
	outputBytes, err := json.Marshal(out)
	if err != nil {
		pdk.SetError(fmt.Errorf("mutator-go: marshal MutatorOutput: %w", err))
		return 1
	}

	pdk.Output(outputBytes)
	return 0
}

func main() {}
