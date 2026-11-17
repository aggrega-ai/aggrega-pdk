// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// Label is one key and value of a drafted entity's metadata.labels.
type Label struct{ Key, Value string }

// SchemaNode is one key of a compact-form pillar: a Component's values, infra
// or provides, or a nested key of one of them.
type SchemaNode struct {
	// Name is the key as written.
	Name string
	// Comment holds the lines written above the key, each through CommentLine.
	Comment []string
	// Type is one type, or several for a union; empty means any value.
	Type []string
	// Description is written as a quoted scalar.
	Description string
	// Required and Sensitive are written when true.
	Required, Sensitive bool
	// Enum holds the allowed values, each a JSON value.
	Enum []json.RawMessage
	// Pattern is written as a quoted scalar.
	Pattern string
	// Min and Max are the compact bounds, which the node's type decides to be
	// value bounds or length bounds.
	Min, Max *json.Number
	// MinLength and MaxLength bound a string's length.
	MinLength, MaxLength *int
	// Format is written as a quoted scalar.
	Format string
	// Items describes the elements of an array; it writes neither its Name nor
	// its Comment.
	Items *SchemaNode
	// Properties are written under properties:, and the node is closed unless
	// Open is set.
	Properties []SchemaNode
	// Open writes additionalProperties: true.
	Open bool
}

// flowOnly reports whether the node carries nothing but its type, so that it
// is written in flow form.
func (n SchemaNode) flowOnly() bool {
	return n.Description == "" && !n.Required && !n.Sensitive && len(n.Enum) == 0 &&
		n.Pattern == "" && n.Min == nil && n.Max == nil && n.MinLength == nil && n.MaxLength == nil &&
		n.Format == "" && n.Items == nil && len(n.Properties) == 0 && !n.Open
}

// flowType writes the node's type as the flow mapping { type: <t> }, or {} for
// a node of any type.
func flowType(types []string) string {
	switch len(types) {
	case 0:
		return "{}"
	case 1:
		return "{ type: " + types[0] + " }"
	default:
		return "{ type: [" + strings.Join(types, ", ") + "] }"
	}
}

// writeSchemaNode writes the node as the key it names, its comment lines above
// it, every line starting at indent.
func writeSchemaNode(b *strings.Builder, indent string, n SchemaNode) {
	for _, line := range n.Comment {
		b.WriteString(CommentLine(indent, singleLine(line)))
	}
	if n.flowOnly() {
		b.WriteString(indent + n.Name + ": " + flowType(n.Type) + "\n")
		return
	}
	b.WriteString(indent + n.Name + ":\n")
	writeSchemaBody(b, indent+"  ", n)
}

// writeSchemaBody writes the keys of a node in block form, in the order type,
// description, required, sensitive, enum, pattern, min, max, minLength,
// maxLength, format, items, properties, additionalProperties.
func writeSchemaBody(b *strings.Builder, indent string, n SchemaNode) {
	switch len(n.Type) {
	case 0:
	case 1:
		b.WriteString(indent + "type: " + n.Type[0] + "\n")
	default:
		b.WriteString(indent + "type: [" + strings.Join(n.Type, ", ") + "]\n")
	}
	if n.Description != "" {
		b.WriteString(indent + "description: " + QuotedString(n.Description) + "\n")
	}
	if n.Required {
		b.WriteString(indent + "required: true\n")
	}
	if n.Sensitive {
		b.WriteString(indent + "sensitive: true\n")
	}
	if len(n.Enum) > 0 {
		values := make([]string, len(n.Enum))
		for i, raw := range n.Enum {
			values[i] = enumValue(raw)
		}
		b.WriteString(indent + "enum: [" + strings.Join(values, ", ") + "]\n")
	}
	if n.Pattern != "" {
		b.WriteString(indent + "pattern: " + QuotedString(n.Pattern) + "\n")
	}
	if n.Min != nil {
		b.WriteString(indent + "min: " + n.Min.String() + "\n")
	}
	if n.Max != nil {
		b.WriteString(indent + "max: " + n.Max.String() + "\n")
	}
	if n.MinLength != nil {
		b.WriteString(indent + "minLength: " + strconv.Itoa(*n.MinLength) + "\n")
	}
	if n.MaxLength != nil {
		b.WriteString(indent + "maxLength: " + strconv.Itoa(*n.MaxLength) + "\n")
	}
	if n.Format != "" {
		b.WriteString(indent + "format: " + QuotedString(n.Format) + "\n")
	}
	if n.Items != nil {
		if n.Items.flowOnly() {
			b.WriteString(indent + "items: " + flowType(n.Items.Type) + "\n")
		} else {
			b.WriteString(indent + "items:\n")
			writeSchemaBody(b, indent+"  ", *n.Items)
		}
	}
	if len(n.Properties) > 0 {
		b.WriteString(indent + "properties:\n")
		for _, p := range n.Properties {
			writeSchemaNode(b, indent+"  ", p)
		}
	}
	switch {
	case n.Open:
		b.WriteString(indent + "additionalProperties: true\n")
	case len(n.Properties) > 0:
		b.WriteString(indent + "additionalProperties: false\n")
	}
}

// enumValue writes one JSON value of an enum as a YAML flow scalar: a string
// through QuotedString, any other value as its compact JSON, which a YAML
// reader takes as the same value.
func enumValue(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return QuotedString(s)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return QuotedString(string(raw))
	}
	return compact.String()
}

// singleLine writes text with every line break as a space, so it stays on the
// one comment line it is written to.
func singleLine(text string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(text)
}
