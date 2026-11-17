// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSchemaNodeWritesFlowAndBlockForms holds a node carrying only a type to
// the flow form and every other node to the block form, keys in their order.
func TestSchemaNodeWritesFlowAndBlockForms(t *testing.T) {
	minLen, maxLen := 1, 9
	lo, hi := json.Number("0"), json.Number("10")
	full := SchemaNode{
		Name: "n", Type: []string{"string"}, Description: "d", Required: true, Sensitive: true,
		Enum:    []json.RawMessage{json.RawMessage(`"a"`), json.RawMessage(`2`)},
		Pattern: "^a", Min: &lo, Max: &hi, MinLength: &minLen, MaxLength: &maxLen, Format: "date",
		Items:      &SchemaNode{Type: []string{"string"}},
		Properties: []SchemaNode{{Name: "p", Type: []string{"integer"}}},
	}
	cases := []struct {
		name string
		node SchemaNode
		want string
	}{
		{"flow", SchemaNode{Name: "a", Type: []string{"string"}}, "  a: { type: string }\n"},
		{"any", SchemaNode{Name: "a"}, "  a: {}\n"},
		{"union flow", SchemaNode{Name: "a", Type: []string{"string", "integer"}}, "  a: { type: [string, integer] }\n"},
		{"open", SchemaNode{Name: "a", Type: []string{"object"}, Open: true}, "  a:\n    type: object\n    additionalProperties: true\n"},
		{"closed", SchemaNode{Name: "a", Type: []string{"object"}, Properties: []SchemaNode{{Name: "b", Type: []string{"string"}}}},
			"  a:\n    type: object\n    properties:\n      b: { type: string }\n    additionalProperties: false\n"},
		{"comment", SchemaNode{Name: "a", Type: []string{"string"}, Comment: []string{"one", "two {{"}},
			"  # one\n  # two {{`{{`}}\n  a: { type: string }\n"},
		{"full", full, "  n:\n    type: string\n    description: \"d\"\n    required: true\n    sensitive: true\n" +
			"    enum: [\"a\", 2]\n    pattern: \"^a\"\n    min: 0\n    max: 10\n    minLength: 1\n    maxLength: 9\n" +
			"    format: \"date\"\n    items: { type: string }\n    properties:\n      p: { type: integer }\n    additionalProperties: false\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b strings.Builder
			writeSchemaNode(&b, "  ", c.node)
			if b.String() != c.want {
				t.Errorf("got\n%q\nwant\n%q", b.String(), c.want)
			}
			var doc map[string]any
			if err := yaml.Unmarshal([]byte(b.String()), &doc); err != nil {
				t.Errorf("does not read back: %v\n%s", err, b.String())
			}
		})
	}
}

// TestSchemaNodeWritesHostileDescriptions reads descriptions back through a
// YAML reader and holds each to the text written.
func TestSchemaNodeWritesHostileDescriptions(t *testing.T) {
	for _, desc := range []string{
		"uses {{ braces", "a # not a comment", "[vendor schema text] leading bracket", "---", "key: value",
		"carriage\rreturn", "trailing backslash\\", "two\nlines", "tab\there", "\"quoted\"", "ünïcode ☃ 😀",
	} {
		var b strings.Builder
		writeSchemaNode(&b, "", SchemaNode{Name: "k", Type: []string{"string"}, Description: desc})
		var doc map[string]struct {
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(b.String()), &doc); err != nil {
			t.Errorf("%q: %v\n%s", desc, err, b.String())
			continue
		}
		if got := doc["k"].Description; got != desc {
			t.Errorf("description %q read back as %q", desc, got)
		}
	}
}

// TestSingleLine holds text carrying line breaks and control characters to one
// line of text a YAML reader reads as a comment.
func TestSingleLine(t *testing.T) {
	for in, want := range map[string]string{
		"a\r\nb": "a b", "a\nb\rc": "a b c", "nel\u0085x": "nel x", "ls ps ": "ls ps ",
		"ctl\x01\x7f": "ctl��", "tab\tkept": "tab\tkept", "plain": "plain",
	} {
		if got := singleLine(in); got != want {
			t.Errorf("singleLine(%q) = %q, want %q", in, got, want)
		}
	}
}
