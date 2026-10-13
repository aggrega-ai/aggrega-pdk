// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import "github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"

// This file holds what the scorer weighs a node against: the candidate
// components and the mapping lookup.
//
// The candidates are the Components of the workspace's own catalog, read from the
// view of that catalog the import input carries (ImportInput.Catalog) — an
// enterprise catalog's Components as much as the standard library's, and nothing
// compiled into the importer. The host builds that view from the catalog the
// importer was selected from and copies each Component's name, labels and key
// names into it without reading what any of them means; reading them is the
// importer's, because domain-specific intelligence is the importer's to hold. So
// the category a candidate belongs to is read here, from its category label, and
// the scorer compares its labels and key names with what a node implies.
//
// Which component a node type maps to is the format's mapping arm, carried in the
// importer. Operators extend or override it through the import attachment's
// settings (mapping_overrides); the arm is the default the overrides layer over.

// categoryLabel is the label a Component's category is read from and written
// under: a candidate's category is read from it, and a drafted Component's
// category is written to it.
const categoryLabel = "component.aggrega.ai/category"

// candidate is one catalog Component as the scorer weighs it: its name, the
// category its category label carries (empty where it carries none), its labels,
// and the top-level key names its values declare.
type candidate struct {
	name     string
	category string
	labels   map[string]string
	values   []string
}

// candidatesFrom reads the candidates out of the catalog view an import input
// carries, one per Component, in the view's own order. An absent view, or one
// holding no Component, yields none — every node is then drafted and none is
// referenced.
//
// What a candidate scores depends on its entry alone, and the scorer orders the
// candidates by score and then by name, so the order of the view never decides
// which candidate ranks first.
func candidatesFrom(view *aggrega.CatalogProjection) []candidate {
	if view == nil {
		return nil
	}
	out := make([]candidate, 0, len(view.Components))
	for _, c := range view.Components {
		out = append(out, candidate{
			name:     c.Name,
			category: c.Labels[categoryLabel],
			labels:   c.Labels,
			values:   c.Values,
		})
	}
	return out
}

// mappingTable answers which mapping a node type has for one import: the
// operator's overrides for the requested format first (override precedence), and
// the served format's own arm second. The overrides layer over the arm so an
// operator can add a node type the arm does not cover or redirect one it does,
// without rebuilding the importer. The arm is consulted only when the request
// names the format the importer serves.
type mappingTable struct {
	overrides mappingOverrides
	format    string
	arm       map[string]Mapping
}

// newMappingTable builds the lookup for one import of format by an importer
// serving the format served describes.
func newMappingTable(overrides mappingOverrides, format string, served Format) mappingTable {
	t := mappingTable{overrides: overrides, format: format}
	if format == served.Name {
		t.arm = served.Mappings
	}
	return t
}

// lookup returns the mapping for a node type, or false when neither the
// overrides nor the arm map it.
func (t mappingTable) lookup(nodeType string) (Mapping, bool) {
	if arm, ok := t.overrides[t.format]; ok {
		if e, ok := arm[nodeType]; ok {
			return e, true
		}
	}
	if e, ok := t.arm[nodeType]; ok {
		return e, true
	}
	return Mapping{}, false
}
