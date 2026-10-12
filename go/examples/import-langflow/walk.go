// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import "encoding/json"

// sourceNode is one node lifted from a parsed source tree: its identifier, the
// type the source names it, and the field and secret names its parameters carry.
// The walker produces these from the canonical tree the host returned; the scorer
// and emitter read them, never the raw source.
type sourceNode struct {
	id      string
	typ     string
	fields  []string // parameter field names, in the order the tree presents them
	secrets []string // the subset of fields whose names read as credentials
}

// sourceEdge is one directed dependency lifted from a parsed source tree: the
// source node feeds the target node, so the target consumes the source.
type sourceEdge struct {
	source string
	target string
}

// walkTree lifts the nodes and edges out of a canonical source tree for a known
// format. It reads the already-parsed tree the host returned — it runs no grammar
// of its own. langflow nests its graph under a top-level data object; flowise
// keeps the graph flat at the top level. An unrecognised shape yields no nodes,
// which the caller surfaces as a diagnostic rather than a silent empty import.
func walkTree(format string, tree json.RawMessage) ([]sourceNode, []sourceEdge) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(tree, &top); err != nil {
		return nil, nil
	}

	var nodesRaw, edgesRaw json.RawMessage
	switch format {
	case "langflow":
		var data map[string]json.RawMessage
		if err := json.Unmarshal(top["data"], &data); err == nil {
			nodesRaw = data["nodes"]
			edgesRaw = data["edges"]
		}
	case "flowise":
		nodesRaw = top["nodes"]
		edgesRaw = top["edges"]
	}

	return parseNodes(format, nodesRaw), parseEdges(edgesRaw)
}

func parseNodes(format string, raw json.RawMessage) []sourceNode {
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	nodes := make([]sourceNode, 0, len(arr))
	for _, n := range arr {
		var outer string
		_ = json.Unmarshal(n["type"], &outer)
		if isCanvasNote(format, outer) {
			continue
		}
		var id string
		_ = json.Unmarshal(n["id"], &id)
		var data map[string]json.RawMessage
		_ = json.Unmarshal(n["data"], &data)

		var typ string
		fields := []string(nil)
		switch format {
		case "langflow":
			_ = json.Unmarshal(data["type"], &typ)
			fields = templateFields(nestedTemplate(data["node"]))
		case "flowise":
			_ = json.Unmarshal(data["name"], &typ)
			fields = subtract(orderedKeys(data["inputs"]), anchorNames(data["inputAnchors"]))
		}
		nodes = append(nodes, sourceNode{id: id, typ: typ, fields: fields, secrets: secretFields(fields)})
	}
	return nodes
}

// isCanvasNote reports whether a node's outer type marks it as a note an author
// pinned to the flow's canvas — langflow's noteNode, flowise's stickyNote. A note
// documents the flow and runs nothing, so it is not lifted as a node: lifting one
// would draft a component for every note. An edge naming a note is then dropped,
// as is any edge naming a node that was not lifted.
func isCanvasNote(format, outerType string) bool {
	switch format {
	case "langflow":
		return outerType == "noteNode"
	case "flowise":
		return outerType == "stickyNote"
	default:
		return false
	}
}

// nestedTemplate returns the template object of a langflow node's data.node,
// where the node's parameter fields live.
func nestedTemplate(nodeRaw json.RawMessage) json.RawMessage {
	var node map[string]json.RawMessage
	if err := json.Unmarshal(nodeRaw, &node); err != nil {
		return nil
	}
	return node["template"]
}

// templateFields returns the names of a langflow template's parameter fields, in
// the order the tree presents them. A template also carries entries that are not
// parameters — a string naming the template's own kind, a boolean refresh flag,
// an object holding only a value the editor keeps for itself — and only an entry
// whose value is an object carrying a type is a parameter field.
func templateFields(raw json.RawMessage) []string {
	var fields []string
	for _, e := range jsonObjectEntries(raw) {
		var field map[string]json.RawMessage
		if err := json.Unmarshal(e.value, &field); err != nil {
			continue
		}
		if _, ok := field["type"]; ok {
			fields = append(fields, e.key)
		}
	}
	return fields
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

func parseEdges(raw json.RawMessage) []sourceEdge {
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	edges := make([]sourceEdge, 0, len(arr))
	for _, e := range arr {
		var src, tgt string
		_ = json.Unmarshal(e["source"], &src)
		_ = json.Unmarshal(e["target"], &tgt)
		if src != "" && tgt != "" {
			edges = append(edges, sourceEdge{source: src, target: tgt})
		}
	}
	return edges
}

// orderedKeys returns the keys of a JSON object in the order they appear in the
// canonical tree. The host's JSON parser sorts object keys, so this order is
// deterministic across runs of the same source.
func orderedKeys(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	return jsonObjectKeys(raw)
}

// secretFields returns the subset of field names that read as credentials, by a
// small name heuristic. The importer never inspects field values — the AOT
// boundary means it holds no resolved secrets — so it classifies on the name
// alone.
func secretFields(fields []string) []string {
	var out []string
	for _, f := range fields {
		if isSecretName(f) {
			out = append(out, f)
		}
	}
	return out
}

func isSecretName(name string) bool {
	for _, needle := range []string{"api_key", "apikey", "api-key", "token", "password", "secret", "credential"} {
		if containsFold(name, needle) {
			return true
		}
	}
	return false
}
