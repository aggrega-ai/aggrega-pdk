// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// Entry is one ordered key of a Recipe map (locals, provides, assertions,
// artifacts): a single-line Value is written quoted, a multi-line one as a
// literal block. The Value is written as given: a Recipe's values are template
// text the compiler evaluates once the Recipe is read, so the caller composes
// that text deliberately and the writer adds no template escape to it.
type Entry struct {
	Key, Value string
	// Comment holds the lines written above the key, each through CommentLine.
	Comment []string
}

// RecipeDraft is what DraftRecipe writes a Recipe from.
type RecipeDraft struct {
	Name, Version, Description, Capability string
	// Implements is the Component the Recipe realizes, <component>:<version>.
	Implements string
	// Header holds the comment lines opening spec, each through CommentLine.
	Header []string
	// Source is spec.source; empty writes none.
	Source string
	// Locals, Provides, Assertions and Artifacts are the maps of spec, and
	// BoundaryArtifacts the maps of spec.boundary.artifacts, each in the order
	// given; an empty one is not written.
	Locals, Provides, Assertions, Artifacts, BoundaryArtifacts []Entry
	// ValuesMapping is spec.valuesMapping, written as a literal block; empty
	// writes none.
	ValuesMapping string
	// Files are written beside the Recipe, by CatalogFiles.
	Files []RecipeFile
}

// DraftRecipe writes the Recipe d describes as deterministic YAML: its keys in
// a fixed order, the parts that are empty not written, and each map in the
// order given, so identical input gives identical bytes. It is the one writer of
// a drafted Recipe for every importer drafting one.
func DraftRecipe(d RecipeDraft) CatalogDraft {
	var b strings.Builder
	b.WriteString("apiVersion: " + apiVersion + "\n")
	b.WriteString("kind: Recipe\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: " + scalar(d.Name) + "\n")
	b.WriteString("  version: " + scalar(d.Version) + "\n")
	if d.Description != "" {
		b.WriteString("  description: " + descriptionScalar(d.Description) + "\n")
	}

	if len(d.Header) == 0 && d.Capability == "" && d.Implements == "" && d.Source == "" &&
		len(d.Locals) == 0 && len(d.Provides) == 0 && len(d.Assertions) == 0 && len(d.Artifacts) == 0 &&
		len(d.BoundaryArtifacts) == 0 && d.ValuesMapping == "" {
		b.WriteString("spec: {}\n")
	} else {
		b.WriteString("spec:\n")
		for _, line := range d.Header {
			b.WriteString(CommentLine("  ", singleLine(line)))
		}
		if d.Capability != "" {
			b.WriteString("  capability: " + scalar(d.Capability) + "\n")
		}
		if d.Implements != "" {
			b.WriteString("  implements: " + scalar(d.Implements) + "\n")
		}
		if d.Source != "" {
			b.WriteString("  source: " + scalar(d.Source) + "\n")
		}
		writeEntries(&b, "  ", "locals", d.Locals)
		writeEntries(&b, "  ", "provides", d.Provides)
		writeEntries(&b, "  ", "assertions", d.Assertions)
		writeEntries(&b, "  ", "artifacts", d.Artifacts)
		if len(d.BoundaryArtifacts) > 0 {
			b.WriteString("  boundary:\n")
			writeEntries(&b, "    ", "artifacts", d.BoundaryArtifacts)
		}
		if d.ValuesMapping != "" {
			b.WriteString("  valuesMapping: " + textValue(d.ValuesMapping, 2, true) + "\n")
		}
	}

	return CatalogDraft{
		Entity:     aggrega.EmittedEntity{Kind: "Recipe", Name: d.Name, Mode: "draft", Body: b.String()},
		Version:    d.Version,
		Capability: d.Capability,
		Files:      d.Files,
	}
}

// writeEntries writes entries as the map named name, whose key line sits at
// indent; nothing is written for no entries.
func writeEntries(b *strings.Builder, indent, name string, entries []Entry) {
	if len(entries) == 0 {
		return
	}
	b.WriteString(indent + name + ":\n")
	keyIndent := indent + "  "
	for _, e := range entries {
		for _, line := range e.Comment {
			b.WriteString(CommentLine(keyIndent, singleLine(line)))
		}
		b.WriteString(keyIndent + scalar(e.Key) + ": " + textValue(e.Value, len(keyIndent), false) + "\n")
	}
}

// textValue writes text as the value of a key whose line sits at keyIndent
// spaces: a literal block where the text holds a line break, or always where
// block is set, and a quoted scalar otherwise. Text a literal block cannot carry
// as written, and text of line feeds alone, which a literal block reads back as
// empty, is quoted, so any text reads back exactly. The result ends without
// a line feed.
func textValue(text string, keyIndent int, block bool) string {
	if (block || strings.Contains(text, "\n")) && strings.Trim(text, "\n") != "" && len(blockProblems(text)) == 0 {
		return strings.TrimSuffix(literalBlock(text, keyIndent), "\n")
	}
	return QuotedString(text)
}

// plainWord matches the text that may be written as a plain scalar: letters,
// digits and . / _ : -, opening with a letter, a digit or one of . / _.
var plainWord = regexp.MustCompile(`^[A-Za-z0-9_./][A-Za-z0-9_./:-]*$`)

// timestampLike matches the text a YAML reader may take for a date.
var timestampLike = regexp.MustCompile(`^[0-9]{4}-[0-9]`)

// scalar writes s as a plain scalar where it reads back as the string s, and as
// a quoted scalar otherwise: a name, a version, a capability, a reference, a
// path or a key stays as readable as the catalog's own Recipes write it, and
// any other text stays exact.
func scalar(s string) string {
	if !plainWord.MatchString(s) || strings.HasSuffix(s, ":") || timestampLike.MatchString(s) || yamlKeyword(s) {
		return QuotedString(s)
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return QuotedString(s)
	}
	if _, err := strconv.ParseInt(s, 0, 64); err == nil {
		return QuotedString(s)
	}
	return s
}

// yamlKeyword reports whether a YAML reader gives the plain scalar s a meaning
// other than a string.
func yamlKeyword(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false", "null", "yes", "no", "on", "off", "y", "n", "~", ".inf", ".nan", ".infinity":
		return true
	}
	return false
}
