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
	// Platform describes the platform a flow of the format runs on, for a format
	// whose sources are flows a platform runs as they are written. A format
	// carrying one is drafted as that platform carrying each source, unless the
	// layout setting asks for nodes; nil drafts every source node by node.
	Platform *Platform
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
	// to, such as core/ai-model/openai. The table signal goes to the catalog
	// Component of that name, so a row naming a Component the request's catalog
	// view does not hold draws no candidate's table signal.
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
// it into nodes and edges, score every node against the Components of the
// catalog view the request carries (in.Catalog), decide each node's disposition
// from its top candidate and the reuse mode, and render the shared decision
// record into files through the selected renderer.
//
// The candidates are the catalog view's Components and nothing else, so a request
// carrying no view, or one holding no Component, scores every node against
// nothing: each is drafted at the NONE tier and none is referenced.
//
// A source that does not parse is reported as a halting diagnostic and one that
// yields no nodes as a warning; the import carries on with the other sources. A
// request naming a format other than served.Name has no walker and no mapping
// arm here, so each of its sources yields no nodes.
//
// Where served describes a Platform and the layout setting does not ask for
// nodes, each source is drafted as that platform carrying it instead, as
// [Platform] describes; the scoring and the dispositions above decide which of
// its nodes become Components of their own. The attach_egress_allowlist setting
// attaches core/egress-allowlist to each platform instance that draft writes; a
// draft made node by node attaches nothing, and warns so where the setting is
// true.
func Import(in aggrega.ImportInput, served Format, parse func(syntax string, source []byte) (json.RawMessage, error)) aggrega.ImportOutput {
	settings := parseSettings(in.Settings)
	if served.Platform != nil && in.Format == served.Name && !settings.nodeLayout() {
		return importPlatform(in, served, settings, parse)
	}
	format := in.Format
	table := newMappingTable(settings.MappingOverrides, format, served)
	catalog := candidatesFrom(in.Catalog)

	sources := sourcesOf(in)
	multiSource := len(sources) > 1

	var out aggrega.ImportOutput
	if settings.AttachEgressAllowlist {
		out.Diagnostics = append(out.Diagnostics, egressAllowlistNotAttached(served.Platform != nil && format == served.Name))
	}
	agg := newAggregateBuilder(settings, format)
	usedNames := map[string]bool{}

	for _, src := range sources {
		tree, err := parse(served.Syntax, src.Bytes)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, parseFailed(src, format, err))
			continue
		}
		nodes, edges := walk(served, format, tree)
		if len(nodes) == 0 {
			out.Diagnostics = append(out.Diagnostics, noNodes(src, format))
			continue
		}

		idToKey := map[string]string{}
		for _, node := range nodes {
			ref := node.ID
			if multiSource {
				ref = src.Name + "#" + node.ID
			}
			key := UniqueKey(usedNames, node.ID)
			idToKey[node.ID] = key

			decision, entity := decideNode(settings, table, catalog, node, ref)
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

// Sources returns the sources an import request carries, as [Import] reads
// them: its named sources, or, where it carries none, its primary bytes as one
// source with no name. An importer that drafts from its sources itself rather
// than through [Import] reads them through this, so every importer built on the
// package reads one request as one list of sources.
func Sources(in aggrega.ImportInput) []aggrega.ImportSource {
	return sourcesOf(in)
}

// ParseFailed is the diagnostic [Import] reports for a source the parse
// callback could not parse, which halts the import: code parse-failed, naming
// the source, the format and the callback's error. An importer that parses its
// sources itself reports such a source with it, under the same code and in the
// same words.
func ParseFailed(src aggrega.ImportSource, format string, err error) aggrega.Diagnostic {
	return parseFailed(src, format, err)
}

// NoNodes is the warning [Import] reports for a source whose tree held nothing
// the importer drafts from: code no-nodes, naming the source and the format. An
// importer that walks its sources itself reports such a source with it, under
// the same code and in the same words.
func NoNodes(src aggrega.ImportSource, format string) aggrega.Diagnostic {
	return noNodes(src, format)
}

// sourcesOf returns the sources an import request carries: its named sources,
// or, where it carries none, its primary bytes as one source with no name.
func sourcesOf(in aggrega.ImportInput) []aggrega.ImportSource {
	if len(in.Sources) == 0 {
		return []aggrega.ImportSource{{Name: "", Bytes: in.Bytes}}
	}
	return in.Sources
}

// parseFailed reports a source the parse callback could not parse, which halts
// the import.
func parseFailed(src aggrega.ImportSource, format string, err error) aggrega.Diagnostic {
	return aggrega.Diagnostic{
		Severity:  "halt",
		Code:      "parse-failed",
		Message:   fmt.Sprintf("could not parse source %q as %s: %s", src.Name, format, err.Error()),
		SourceRef: src.Name,
	}
}

// noNodes reports a source whose tree the walker lifted no node from.
func noNodes(src aggrega.ImportSource, format string) aggrega.Diagnostic {
	return aggrega.Diagnostic{
		Severity:  "warn",
		Code:      "no-nodes",
		Message:   fmt.Sprintf("source %q carried no recognisable %s nodes", src.Name, format),
		SourceRef: src.Name,
	}
}

// egressAllowlistNotAttached reports that the attach_egress_allowlist setting
// attached nothing, since the draft is made node by node and writes no platform
// to attach it to: because the layout setting asks for nodes, where nodesAsked
// is true, and otherwise because the format describes no platform.
func egressAllowlistNotAttached(nodesAsked bool) aggrega.Diagnostic {
	why := "the format describes no platform"
	if nodesAsked {
		why = "the layout setting is " + layoutNodes
	}
	return aggrega.Diagnostic{
		Severity: "warn",
		Code:     "feature-not-attached",
		Message: fmt.Sprintf("the attach_egress_allowlist setting attaches %s to the platform a draft carries the flow in, "+
			"and this draft writes no platform, since %s, so nothing is attached", egressAllowlist, why),
	}
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
