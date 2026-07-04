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
)

// proofLabelKey is the label key the proof plugin appends. It carries a
// "/" precisely to exercise RFC 6901 escaping end-to-end: the emitted
// patch path escapes it as "~1" so the host adds one label entry keyed by
// the literal "proof.aggrega/touched".
const proofLabelKey = "proof.aggrega/touched"

// proofLabelValue is the label value the proof plugin writes.
const proofLabelValue = "mutator-proof"

// pluginDescriptor is the static describe() self-report: a v1alpha1
// mutator plugin whose mutate export the host validates before dispatch.
const pluginDescriptor = `{"abi_version":"v1alpha1","plugin_type":"mutator","exports":[{"name":"mutate"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}`

// mutatorInput is the minimal shape the proof plugin reads from the
// MutatorInput payload: it needs only the node IDs to root a per-node
// label patch. The full payload carries more (compilation context, per-node
// component / resolved_version / labels), but the proof plugin ignores it.
type mutatorInput struct {
	Graph struct {
		Nodes map[string]json.RawMessage `json:"nodes"`
	} `json:"graph"`
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
	pdk.Output([]byte(pluginDescriptor))
	return 0
}

//go:wasmexport mutate
func mutate() int32 {
	var in mutatorInput
	if err := json.Unmarshal(pdk.Input(), &in); err != nil {
		pdk.SetError(fmt.Errorf("mutator-proof: unmarshal MutatorInput: %w", err))
		return 1
	}

	// Go map iteration is non-deterministic; sorted iteration keeps the
	// emitted patches byte-identical across runs of identical input.
	nodeIDs := make([]string, 0, len(in.Graph.Nodes))
	for nodeID := range in.Graph.Nodes {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)

	patches := make([]json.RawMessage, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		path := fmt.Sprintf("/nodes/%s/labels/%s",
			escapePointerToken(nodeID), escapePointerToken(proofLabelKey))
		patch := fmt.Sprintf(`[{"op":"add","path":%q,"value":%q}]`, path, proofLabelValue)
		patches = append(patches, json.RawMessage(patch))
	}

	out := struct {
		Patches []json.RawMessage `json:"patches"`
	}{Patches: patches}
	outputBytes, err := json.Marshal(out)
	if err != nil {
		pdk.SetError(fmt.Errorf("mutator-proof: marshal MutatorOutput: %w", err))
		return 1
	}
	pdk.Output(outputBytes)
	return 0
}

func main() {}
