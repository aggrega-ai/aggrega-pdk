// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"bytes"
	"encoding/json"
)

// ObjectEntry is one member of a JSON object: its key and its value, undecoded.
type ObjectEntry struct {
	Key   string
	Value json.RawMessage
}

// ObjectEntries returns the members of a JSON object in the textual order the
// document presents them, using a streaming walk so the order is preserved
// (decoding into a Go map would lose it). The host's parser emits object keys in
// sorted order, so this order is deterministic across runs of the same source. A
// document that is not an object yields no entries; one that breaks off yields
// the entries read before the break.
func ObjectEntries(raw json.RawMessage) []ObjectEntry {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	var entries []ObjectEntry
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return entries
		}
		key, ok := keyTok.(string)
		if !ok {
			return entries
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return entries
		}
		entries = append(entries, ObjectEntry{Key: key, Value: value})
	}
	return entries
}

// NodeList lifts the nodes of a JSON array of node objects, each carrying its
// identifier as id, its outer type as type, and its parameters under a data
// object — the shape a flow editor's canvas export lists its nodes in. Each
// element's id, outer type and data object are decoded and handed to lift, which
// reads the node's type and parameter fields from the data in the source format's
// own way and reports whether the element is a node at all: an element it declines,
// such as a note pinned to the canvas, is not lifted. A member that is absent or
// does not decode is handed over empty. The nodes keep the array's order.
//
// A raw value that is not an array yields no nodes.
func NodeList(raw json.RawMessage, lift func(id, outerType string, data map[string]json.RawMessage) (Node, bool)) []Node {
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	nodes := make([]Node, 0, len(arr))
	for _, n := range arr {
		var outer string
		_ = json.Unmarshal(n["type"], &outer)
		var id string
		_ = json.Unmarshal(n["id"], &id)
		var data map[string]json.RawMessage
		_ = json.Unmarshal(n["data"], &data)
		if node, ok := lift(id, outer, data); ok {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// EdgeList lifts the edges of a JSON array of edge objects, each naming the node
// it leaves as source and the node it enters as target — the shape a flow
// editor's canvas export lists its edges in. An element whose source or target
// is absent, empty or not a string is skipped. The edges keep the array's order.
//
// A raw value that is not an array yields no edges.
func EdgeList(raw json.RawMessage) []Edge {
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	edges := make([]Edge, 0, len(arr))
	for _, e := range arr {
		var src, tgt string
		_ = json.Unmarshal(e["source"], &src)
		_ = json.Unmarshal(e["target"], &tgt)
		if src != "" && tgt != "" {
			edges = append(edges, Edge{Source: src, Target: tgt})
		}
	}
	return edges
}
