// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import (
	"encoding/json"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/importer"
)

// walkTree lifts the nodes and edges out of the canonical tree of a Flowise
// flow. It reads the already-parsed tree the host returned — it runs no grammar
// of its own. Flowise keeps its graph flat at the top level. An unrecognised
// shape yields no nodes, which the importer surfaces as a diagnostic rather than
// a silent empty import.
func walkTree(tree json.RawMessage) ([]importer.Node, []importer.Edge) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(tree, &top); err != nil {
		return nil, nil
	}
	return importer.NodeList(top["nodes"], liftNode), importer.EdgeList(top["edges"])
}

// liftNode reads one element of a Flowise node list: the node's type from its
// data's name, its credential parameters from its inputParams, and its value
// fields from its inputs, less the inputs another node connects into and the
// credentials. A note pinned to the canvas is not lifted.
func liftNode(id, outerType string, data map[string]json.RawMessage) (importer.Node, bool) {
	if isCanvasNote(outerType) {
		return importer.Node{}, false
	}
	var typ string
	_ = json.Unmarshal(data["name"], &typ)
	secrets := credentialParams(data["inputParams"])
	fields := subtract(subtract(orderedKeys(data["inputs"]), anchorNames(data["inputAnchors"])), secrets)
	return importer.Node{ID: id, Type: typ, Fields: fields, Secrets: secrets}, true
}

// isCanvasNote reports whether a node's outer type marks it as a note an author
// pinned to the flow's canvas — Flowise's stickyNote. A note documents the flow and
// runs nothing, so it is not lifted as a node: lifting one would draft a component
// for every note. An edge naming a note is then dropped, as is any edge naming a
// node that was not lifted.
func isCanvasNote(outerType string) bool {
	return outerType == "stickyNote"
}

// credentialParams returns the names of a flowise node's credential parameters,
// in the order its inputParams list them. Flowise declares a node's credential as
// an input parameter of type credential, and keeps the credential itself out of
// the node's inputs, so the parameter is the only place the node says it needs
// one. Only each parameter's name and type are read; an entry that does not
// decode is skipped without affecting the others.
func credentialParams(raw json.RawMessage) []string {
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		var param struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(entry, &param); err != nil {
			continue
		}
		if param.Type == "credential" && param.Name != "" {
			names = append(names, param.Name)
		}
	}
	return names
}

// anchorNames returns the names a flowise node's inputAnchors declare. An anchor
// is where another node connects in — its entry in the node's inputs holds a
// reference to that node, which the edge between the two already records — so
// an anchor is a connection rather than a parameter field.
func anchorNames(raw json.RawMessage) []string {
	var anchors []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &anchors); err != nil {
		return nil
	}
	names := make([]string, 0, len(anchors))
	for _, a := range anchors {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return names
}

// orderedKeys returns the keys of a JSON object in the order they appear in the
// canonical tree. The host's JSON parser sorts object keys, so this order is
// deterministic across runs of the same source.
func orderedKeys(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	entries := importer.ObjectEntries(raw)
	if entries == nil {
		return nil
	}
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, e.Key)
	}
	return keys
}

// subtract returns the elements of a not present in b, preserving a's order.
func subtract(a, b []string) []string {
	drop := make(map[string]struct{}, len(b))
	for _, s := range b {
		drop[s] = struct{}{}
	}
	var out []string
	for _, s := range a {
		if _, ok := drop[s]; !ok {
			out = append(out, s)
		}
	}
	return out
}
