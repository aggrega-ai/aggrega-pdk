// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// End-to-end proof mutator plugin for the Aggrega Wasm runtime.
//
// Unlike the minimal mutator fixture, this plugin emits JSON Patch
// documents that APPLY cleanly to the host's labels-only projection: it
// escapes the label key per RFC 6901 (a "/" in a key becomes "~1"), so
// the host adds a single label entry rather than failing on a
// non-existent intermediate path. It exists so the host-side end-to-end
// test can prove the Step-2 mutator socket injects a label all the way
// through to the IR graph.
//
// It self-describes via describe() as a mutator plugin whose mutate
// export the host validates before dispatch, then appends the label
// "proof.aggrega/touched=mutator-proof" to every node.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o mutator-proof-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// proofLabelKey is the label key the proof plugin appends. It carries a
// "/" precisely to exercise RFC 6901 escaping end-to-end: the emitted
// patch path escapes it as "~1" so the host adds one label entry keyed by
// the literal "proof.aggrega/touched".
const proofLabelKey = "proof.aggrega/touched"

// proofLabelValue is the label value the proof plugin writes.
const proofLabelValue = "mutator-proof"

// descriptor is the static describe() self-report: a v1alpha1 mutator plugin
// whose mutate export the host validates before dispatch.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion: aggrega.APIVersion,
		PluginType: "governance",
		Exports:    []aggrega.PluginExport{{Name: "mutate"}},
		Metadata:   &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
}

// escapePointerToken escapes a JSON Pointer reference token per RFC 6901:
// "~" becomes "~0" and "/" becomes "~1". The order matters — "~" is
// escaped first so a "/" rewritten to "~1" is not re-escaped.
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

// mutate reads the label-transform payload and reports one label addition per
// node. The payload and the result travel as canonical CBOR, decoded and encoded
// by the generated codec on the shared payload type — the plugin reads only the
// node IDs, but it reads them through the wire shape rather than a local
// restatement of it, so a change to that shape reaches this fixture.
//
//go:wasmexport mutate
func mutate() int32 {
	var in aggrega.MutatorInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(fmt.Errorf("mutator-proof: decode MutatorInput: %w", err))
		return 1
	}

	// The nodes arrive in an aggrega.OrderedMap, in the order the host sent
	// them; sorted iteration makes the emitted patches independent of that
	// order, so identical input yields byte-identical output.
	nodeIDs := in.Graph.Nodes.Keys()
	sort.Strings(nodeIDs)

	patches := make([]json.RawMessage, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		path := fmt.Sprintf("/nodes/%s/labels/%s",
			escapePointerToken(nodeID), escapePointerToken(proofLabelKey))
		patch := fmt.Sprintf(`[{"op":"add","path":%q,"value":%q}]`, path, proofLabelValue)
		patches = append(patches, json.RawMessage(patch))
	}

	outputBytes, err := aggrega.MutatorOutput{Patches: patches}.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("mutator-proof: encode MutatorOutput: %w", err))
		return 1
	}
	pdk.Output(outputBytes)
	return 0
}

func main() {}
