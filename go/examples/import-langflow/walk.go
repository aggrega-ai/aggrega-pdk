// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import "encoding/json"

// sourceNode is one node lifted from a parsed source tree: its identifier, the
// type the source names it, and the names of the value fields and the credential
// fields its parameters carry. The walker produces these from the canonical tree
// the host returned; the scorer and emitter read them, never the raw source.
//
// The two name lists are disjoint: a parameter the source format marks as a
// credential is in secrets and not in fields, and every other parameter field is
// in fields.
type sourceNode struct {
	id      string
	typ     string
	fields  []string // value field names, in the order the tree presents them
	secrets []string // credential field names, as the source format marks them, in the order the tree presents them
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
		var fields, secrets []string
		switch format {
		case "langflow":
			_ = json.Unmarshal(data["type"], &typ)
			fields, secrets = templateFields(nestedTemplate(data["node"]))
		case "flowise":
			_ = json.Unmarshal(data["name"], &typ)
			secrets = credentialParams(data["inputParams"])
			fields = subtract(subtract(orderedKeys(data["inputs"]), anchorNames(data["inputAnchors"])), secrets)
		}
		nodes = append(nodes, sourceNode{id: id, typ: typ, fields: fields, secrets: secrets})
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
// the order the tree presents them, split into value fields and credential
// fields. A template also carries entries that are not parameters — a string
// naming the template's own kind, a boolean refresh flag, an object holding only
// a value the editor keeps for itself — and only an entry whose value is an
// object carrying a type is a parameter field.
//
// A parameter field is a credential when langflow marks it as one: its object
// carries password set to true, which is how langflow declares a field whose
// value is secret. The name plays no part, so a field named like a credential
// that langflow does not mark is a value field, and a marked field is a
// credential whatever it is called. Only the type and password members are read;
// the field's value is never decoded.
func templateFields(raw json.RawMessage) (fields, secrets []string) {
	for _, e := range jsonObjectEntries(raw) {
		var field struct {
			Type     json.RawMessage `json:"type"`
			Password json.RawMessage `json:"password"`
		}
		if err := json.Unmarshal(e.value, &field); err != nil || field.Type == nil {
			continue
		}
		if string(field.Password) == "true" {
			secrets = append(secrets, e.key)
		} else {
			fields = append(fields, e.key)
		}
	}
	return fields, secrets
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
