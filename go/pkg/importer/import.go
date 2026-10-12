// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"encoding/json"
	"fmt"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// Format describes the one source format an importer serves: everything
// [Import] needs that depends on the format.
type Format struct {
	// Name is the format's name: the <format> of the import:<format> capability
	// the importer advertises, and the format an import request for this
	// importer names.
	Name string
	// Syntax is the syntactic format a source of this format is parsed as: the
	// format the parse callback is asked for, which the host hands to a parser
	// advertising parse:<syntax>.
	Syntax string
	// Mappings is the format's mapping arm, keyed by the node type the format
	// names. The mapping_overrides setting layers over it.
	Mappings map[string]Mapping
	// Walk lifts the nodes, and the edges between them, out of the canonical
	// tree the parse callback returned for one source. A tree it does not
	// recognise yields no nodes, which [Import] reports as a warning.
	Walk func(tree json.RawMessage) ([]Node, []Edge)
}

// Node is one node a walker lifted from a source tree: its identifier, the type
// the source names it by, and the names of its parameter fields, split into
// value fields and the fields the source format marks as credentials. The two
// lists are disjoint, and each keeps the order the tree presents its fields in.
type Node struct {
	ID      string
	Type    string
	Fields  []string
	Secrets []string
}

// Edge is one directed dependency between two nodes: Source feeds Target, so
// Target consumes Source. An edge naming a node the walker did not lift is
// dropped.
type Edge struct {
	Source string
	Target string
}

// Mapping is one row of a mapping arm: a statement that a node type IS a
// component, the strongest signal the scorer weighs.
type Mapping struct {
	// Component is the fully-qualified name of the component the node type maps
	// to, such as core/ai-model/openai.
	Component string
	// Category is the component.aggrega.ai/category bucket a node of the type
	// belongs to.
	Category string
	// Traits are the refining labels the node type implies, weighed against a
	// candidate's domain labels.
	Traits map[string]string
	// Vendor is the vendor token the node type carries, weighed against the last
	// segment of a candidate's name.
	Vendor string
}

// Import turns one import request into the entities, decisions, diagnostics and
// files it produces, for an importer serving the format served describes. It is
// the pure core of an importer: it takes the decoded request and a parse
// callback — the importer's only channel to a source tree, which the host
// mediates — and returns the assembled output, holding no state and touching no
// ambient resource. An importer's import export wires the host's parse into it;
// a test wires a local one.
//
// The flow is: decode the settings, parse each source as served.Syntax and walk
// it into nodes and edges, score every node against the candidate components,
// decide each node's disposition from its top candidate and the reuse mode, and
// render the shared decision record into files through the selected renderer.
//
// A source that does not parse is reported as a halting diagnostic and one that
// yields no nodes as a warning; the import carries on with the other sources. A
// request naming a format other than served.Name has no walker and no mapping
// arm here, so each of its sources yields no nodes.
func Import(in aggrega.ImportInput, served Format, parse func(syntax string, source []byte) (json.RawMessage, error)) aggrega.ImportOutput {
	settings := parseSettings(in.Settings)
	format := in.Format
	table := newMappingTable(settings.MappingOverrides, format, served)

	sources := in.Sources
	if len(sources) == 0 {
		sources = []aggrega.ImportSource{{Name: "", Bytes: in.Bytes}}
	}
	multiSource := len(sources) > 1

	var out aggrega.ImportOutput
	agg := newAggregateBuilder(settings, format)
	usedNames := map[string]bool{}

	for _, src := range sources {
		tree, err := parse(served.Syntax, src.Bytes)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
				Severity:  "halt",
				Code:      "parse-failed",
				Message:   fmt.Sprintf("could not parse source %q as %s: %s", src.Name, format, err.Error()),
				SourceRef: src.Name,
			})
			continue
		}
		nodes, edges := walk(served, format, tree)
		if len(nodes) == 0 {
			out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
				Severity:  "warn",
				Code:      "no-nodes",
				Message:   fmt.Sprintf("source %q carried no recognisable %s nodes", src.Name, format),
				SourceRef: src.Name,
			})
			continue
		}

		idToKey := map[string]string{}
		for _, node := range nodes {
			ref := node.ID
			if multiSource {
				ref = src.Name + "#" + node.ID
			}
			key := uniqueKey(usedNames, kebab(node.ID))
			idToKey[node.ID] = key

			decision, entity := decideNode(settings, table, node, ref)
			out.Decisions = append(out.Decisions, decision)
			out.Entities = append(out.Entities, entity)
			agg.addComponent(key, decision, entity)
		}
		for _, e := range edges {
			consumer, ok1 := idToKey[e.Target]
			producer, ok2 := idToKey[e.Source]
			if ok1 && ok2 {
				agg.addEdge(consumer, producer)
			}
		}
	}

	// The aggregate wiring the drafted and referenced components together is the
	// last entity, emitted after the components it composes.
	out.Entities = append(out.Entities, agg.entity())

	out.Files = render(settings, out.Entities, out.Decisions)
	return out
}

// walk lifts the nodes and edges out of one parsed source through the served
// format's walker. A request naming another format has no walker here, so it
// yields nothing — the same outcome as a tree the walker does not recognise.
func walk(served Format, format string, tree json.RawMessage) ([]Node, []Edge) {
	if format != served.Name || served.Walk == nil {
		return nil, nil
	}
	return served.Walk(tree)
}
