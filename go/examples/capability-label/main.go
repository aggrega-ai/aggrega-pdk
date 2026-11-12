// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Capability-label plugin for the Aggrega label-transform role.
//
// A capability that depends on how an instance is configured cannot be a label
// a catalog Component carries: a PostgreSQL server is a vector store only where
// its values enable the vector extension, and the catalog cannot know whether
// they will. The developer declares that configuration in the Aggregate's
// values, and the label-transform role hands each node's values to the plugin
// attached there, so the plugin can read the configuration and add the label
// the configuration earns.
//
// This plugin reads one such configuration. A node whose Component is in the
// datastore category, and whose values hold an `extensions` list naming
// `vector`, receives `datastore.aggrega.ai/engine: vector`. A node whose values
// name no such extension is left as it is, and so is a node that already
// carries an engine label: the engine label states what a product is, and a
// product's own statement wins over a configuration's.
//
// The values arrive read-only. The plugin returns JSON Patch documents, and
// the host applies them to the node labels alone, so a patch aimed at a value
// is refused rather than applied.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o capability-label-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

const (
	// categoryLabel and datastoreCategory select the nodes the rule applies to:
	// the extension promotes a datastore, so only a datastore is considered.
	categoryLabel     = "component.aggrega.ai/category"
	datastoreCategory = "datastore"

	// extensionsValue is the value the rule reads: a list of extension names
	// the developer enables on the instance.
	extensionsValue = "extensions"
	// vectorExtension is the extension whose presence promotes the label.
	vectorExtension = "vector"

	// engineLabel and vectorEngine are the capability label the rule adds.
	engineLabel  = "datastore.aggrega.ai/engine"
	vectorEngine = "vector"
)

// descriptor is the static describe() self-report: a governance plugin whose
// mutate export the host validates before dispatch at the label-transform role.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Label a datastore whose values enable the vector extension as a vector engine.",
		Exports:     []aggrega.PluginExport{{Name: "mutate"}},
		Metadata:    &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
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

// promotes reports whether a node's labels and values earn the vector engine
// label: the node is a datastore, carries no engine label of its own, and its
// values hold an extensions list naming the vector extension. A value of
// another shape under that key is not a list of extensions, so it earns nothing
// rather than failing the build.
func promotes(node aggrega.MutatorNode) bool {
	if category, _ := node.Labels.Get(categoryLabel); category != datastoreCategory {
		return false
	}
	if _, labelled := node.Labels.Get(engineLabel); labelled {
		return false
	}
	raw, ok := node.Values.Get(extensionsValue)
	if !ok {
		return false
	}
	var extensions []string
	if err := json.Unmarshal(raw, &extensions); err != nil {
		return false
	}
	return slices.Contains(extensions, vectorExtension)
}

// escapePointerToken escapes a JSON Pointer reference token per RFC 6901: "~"
// becomes "~0" and "/" becomes "~1", in that order, so a node ID or a label key
// carrying a "/" addresses one entry rather than a nested path.
func escapePointerToken(tok string) string {
	tok = strings.ReplaceAll(tok, "~", "~0")
	tok = strings.ReplaceAll(tok, "/", "~1")
	return tok
}

//go:wasmexport mutate
func mutate() int32 {
	var in aggrega.MutatorInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(fmt.Errorf("capability-label: decode MutatorInput: %w", err))
		return 1
	}

	// Sorted node IDs make the patch list independent of the order the nodes
	// arrive in, so identical input yields byte-identical output.
	nodeIDs := in.Graph.Nodes.Keys()
	sort.Strings(nodeIDs)

	patches := make([]json.RawMessage, 0)
	for _, nodeID := range nodeIDs {
		node, _ := in.Graph.Nodes.Get(nodeID)
		if !promotes(node) {
			continue
		}
		path := "/nodes/" + escapePointerToken(nodeID) + "/labels/" + escapePointerToken(engineLabel)
		patch, err := json.Marshal([]map[string]string{{"op": "add", "path": path, "value": vectorEngine}})
		if err != nil {
			pdk.SetError(fmt.Errorf("capability-label: encode patch: %w", err))
			return 1
		}
		patches = append(patches, patch)
	}

	out, err := aggrega.MutatorOutput{Patches: patches}.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("capability-label: encode MutatorOutput: %w", err))
		return 1
	}
	pdk.Output(out)
	return 0
}

func main() {}
