// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"regexp"
	"strings"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// ComponentSlot is one entry of a drafted Component's featuresSchema.
type ComponentSlot struct {
	Name, Selector, Description string
	// Required and Sensitive are written when true.
	Required, Sensitive bool
	// Inject is the delivery mechanisms, written as a flow list; nil writes
	// inject: [].
	Inject []string
}

// ComponentDraft is what DraftComponent writes a Component from.
type ComponentDraft struct {
	Name, Version string
	// Description is written when it is not empty.
	Description string
	Labels      []Label
	// Values, Infra and Provides are the pillars of spec.schema, each written
	// as a closed object holding its nodes under properties; a nil pillar is
	// not written.
	Values, Infra, Provides []SchemaNode
	Slots                   []ComponentSlot
	// Inject is the mechanisms of spec.inject; nil writes none.
	Inject []string
}

// RecipeFile is a file written beside a drafted entity; Path is relative to its
// version directory, slash-separated.
type RecipeFile struct{ Path, Content string }

// CatalogDraft is one drafted catalog entity, its version, its capability (the
// Recipe's) and the files beside it.
type CatalogDraft struct {
	// Entity is the drafted Component or Recipe, in draft mode, named after
	// its metadata.name.
	Entity     aggrega.EmittedEntity
	Version    string
	Capability string
	Files      []RecipeFile
}

// DraftComponent writes the Component d describes as deterministic YAML: its
// keys in a fixed order and its pillars, slots and labels in the order given, so
// identical input gives identical bytes. It is the one writer of a drafted
// Component, for the kit's own drafts and for every importer drafting a
// Component of its own.
func DraftComponent(d ComponentDraft) CatalogDraft {
	var b strings.Builder
	b.WriteString("apiVersion: " + apiVersion + "\n")
	b.WriteString("kind: Component\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: " + d.Name + "\n")
	b.WriteString("  version: " + d.Version + "\n")
	if d.Description != "" {
		b.WriteString("  description: " + descriptionScalar(d.Description) + "\n")
	}
	if len(d.Labels) > 0 {
		b.WriteString("  labels:\n")
		for _, l := range d.Labels {
			b.WriteString("    " + l.Key + ": " + l.Value + "\n")
		}
	}

	if len(d.Slots) == 0 && d.Inject == nil && d.Values == nil && d.Infra == nil && d.Provides == nil {
		b.WriteString("spec: {}\n")
	} else {
		b.WriteString("spec:\n")
		if len(d.Slots) > 0 {
			b.WriteString("  featuresSchema:\n")
			for _, s := range d.Slots {
				writeComponentSlot(&b, s)
			}
		}
		if d.Inject != nil {
			b.WriteString("  inject: " + flowList(d.Inject) + "\n")
		}
		if d.Values != nil || d.Infra != nil || d.Provides != nil {
			b.WriteString("  schema:\n")
			writePillar(&b, "values", d.Values)
			writePillar(&b, "infra", d.Infra)
			writePillar(&b, "provides", d.Provides)
		}
	}

	return CatalogDraft{
		Entity:  aggrega.EmittedEntity{Kind: "Component", Name: d.Name, Mode: "draft", Body: b.String()},
		Version: d.Version,
	}
}

// writeComponentSlot writes one featuresSchema entry, its keys in the order
// selector, sensitive, required, inject, description.
func writeComponentSlot(b *strings.Builder, s ComponentSlot) {
	b.WriteString("    " + s.Name + ":\n")
	if s.Selector != "" {
		b.WriteString("      selector: " + QuotedString(s.Selector) + "\n")
	}
	if s.Sensitive {
		b.WriteString("      sensitive: true\n")
	}
	if s.Required {
		b.WriteString("      required: true\n")
	}
	b.WriteString("      inject: " + flowList(s.Inject) + "\n")
	if s.Description != "" {
		b.WriteString("      description: " + descriptionScalar(s.Description) + "\n")
	}
}

// writePillar writes one pillar of spec.schema as a closed object whose nodes
// sit under an explicit properties block, so a node named like a schema keyword
// is still a field name; a nil pillar writes nothing.
func writePillar(b *strings.Builder, name string, nodes []SchemaNode) {
	if nodes == nil {
		return
	}
	b.WriteString("    " + name + ":\n")
	b.WriteString("      type: object\n")
	if len(nodes) > 0 {
		b.WriteString("      properties:\n")
		for _, n := range nodes {
			writeSchemaNode(b, "        ", n)
		}
	}
	b.WriteString("      additionalProperties: false\n")
}

// flowList writes items as a YAML flow sequence of quoted scalars.
func flowList(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = QuotedString(s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// plainText matches the text a YAML reader takes as the string it reads: words
// of letters and digits with single spaces and the punctuation . , ; _ -, the
// first a letter, and at least one space so that no word is a YAML keyword.
var plainText = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9.,;_-]*( [A-Za-z0-9.,;_-]+)+$`)

// descriptionScalar writes a slot's or a Component's description as a plain
// scalar where its text reads back unchanged as one, and as a quoted scalar
// otherwise, so composed prose stays readable and any other text stays exact.
func descriptionScalar(s string) string {
	if plainText.MatchString(s) {
		return s
	}
	return QuotedString(s)
}
