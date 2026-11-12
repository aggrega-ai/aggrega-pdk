// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Fixture governance plugin for the label-transform role, for the Aggrega Wasm runtime.
//
// Applies one trivial transform — appends the label
// "fixture.aggrega-pdk/touched=mutator-go" to every node in the
// MutatorGraph — and returns one JSON Patch document per node carrying
// that addition. The host can verify end-to-end that:
//
//   - The plugin loaded successfully via the Aggrega Wasm loader.
//   - The canonical CBOR payload decoded to MutatorInput.
//   - The transform applied per node.
//   - The result encoded to a MutatorOutput valid against the
//     wasm-mutator schema.
//
// The label-transform role's wire carries canonical CBOR in both directions. The
// patch documents the output carries are JSON, and stay JSON: the socket carries
// them verbatim and neither side reads into them.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o mutator-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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

// descriptor is the static describe() self-report: a governance plugin
// whose mutate export the host validates before dispatch. The host reads this
// once at load through the shared describe() gate.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Append a fixed label to every node in the mutation graph.",
		Exports:     []aggrega.PluginExport{{Name: "mutate"}},
		Metadata:    &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
}

// escapePointerToken escapes a JSON Pointer reference token per RFC 6901:
// "~" becomes "~0" and "/" becomes "~1". The order matters — "~" is
// escaped first so a "/" rewritten to "~1" is not re-escaped. The label
// key carries a "/", so escaping is required for the emitted patch path
// to address a single label entry rather than a non-existent nested path.
func escapePointerToken(tok string) string {
	tok = strings.ReplaceAll(tok, "~", "~0")
	tok = strings.ReplaceAll(tok, "/", "~1")
	return tok
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

//go:wasmexport mutate
func mutate() int32 {
	inputBytes := pdk.Input()

	var in aggrega.MutatorInput
	if err := in.DecodeCBOR(inputBytes); err != nil {
		pdk.SetError(fmt.Errorf("mutator-go: decode MutatorInput: %w", err))
		return 1
	}

	// Attachment inputs arrive alongside the graph. Settings is a free-form
	// object the host hands through verbatim; here the optional "label_value"
	// key overrides the label the fixture writes, defaulting to the language
	// constant when no settings are attached. Sources carry the attachment's
	// declared definition files as {Path, Content} — a real mutator parses them
	// to drive its labelling; the fixture simply reads them to demonstrate
	// access. Both are absent unless the attachment declared a `with:` block.
	labelValue := fixtureLabelValue
	if len(in.Settings) > 0 {
		var s struct {
			LabelValue string `json:"label_value"`
		}
		if err := json.Unmarshal(in.Settings, &s); err == nil && s.LabelValue != "" {
			labelValue = s.LabelValue
		}
	}
	for _, src := range in.Sources {
		// A real plugin parses src.Content (addressed by src.Path) here.
		_ = src
	}

	// Iterate node IDs in sorted order. The graph's nodes arrive in an
	// aggrega.OrderedMap, in the order the host sent them; sorting the IDs
	// makes the emitted patch list independent of that order, so identical
	// input yields byte-identical output however the nodes were ordered —
	// both to match Aggrega's determinism discipline and to keep this
	// fixture safe to copy as a starting template for real label-transform plugins.
	nodeIDs := in.Graph.Nodes.Keys()
	sort.Strings(nodeIDs)

	patches := make([]json.RawMessage, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		path := fmt.Sprintf("/nodes/%s/labels/%s",
			escapePointerToken(nodeID), escapePointerToken(fixtureLabelKey))
		patch := fmt.Sprintf(`[{"op":"add","path":%q,"value":%q}]`, path, labelValue)
		patches = append(patches, json.RawMessage(patch))
	}

	out := aggrega.MutatorOutput{Patches: patches}
	outputBytes, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("mutator-go: encode MutatorOutput: %w", err))
		return 1
	}

	pdk.Output(outputBytes)
	return 0
}

func main() {}
