// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This file holds the writer of a flow definition into a drafted Aggregate: the
// emptying of the credential values a source holds, the escape of the text an
// Aggregate would otherwise read as a template action, the YAML literal block
// the definition is written as, and the identifier a flow is given where its
// source names none.

// escapeTemplateText writes text so that it reads back as itself once an
// Aggregate's text has been evaluated as a template: every "{{" is written as
// {{"{{"}}, an action whose output is the two braces, and nothing else is
// changed. A closing "}}" outside an action is plain text to the template
// engine and is left as written.
//
// It is the one place a drafted Aggregate's text is escaped. The escaped text
// carries double quotes, so it is written where a double quote is an ordinary
// character, as a literal block scalar is.
func escapeTemplateText(text string) string {
	return strings.ReplaceAll(text, "{{", `{{"{{"}}`)
}

// literalBlock writes text as a YAML literal block scalar, the value of a key
// whose line sits at keyIndent spaces: the header — the block indicator with
// the chomping indicator that reads the text's own trailing line breaks back,
// and an indentation indicator where the text opens with a space or a line
// break — followed by every line of text indented two spaces beyond the key. An
// empty line is written empty.
//
// The text must hold only what blockProblems admits; the caller checks first.
func literalBlock(text string, keyIndent int) string {
	trailing := len(text) - len(strings.TrimRight(text, "\n"))
	header := "|"
	if strings.HasPrefix(text, " ") || strings.HasPrefix(text, "\n") {
		header += "2"
	}
	switch {
	case trailing == 0:
		header += "-"
	case trailing > 1:
		header += "+"
	}
	pad := strings.Repeat(" ", keyIndent+2)
	var b strings.Builder
	b.WriteString(header + "\n")
	for _, line := range strings.SplitAfter(text, "\n") {
		switch line {
		case "":
		case "\n":
			b.WriteString("\n")
		default:
			b.WriteString(pad + line)
		}
	}
	if trailing == 0 {
		b.WriteString("\n")
	}
	return b.String()
}

// blockProblem is one kind of character a YAML literal block scalar cannot
// carry as written, with how often the text holds it and where it first does.
type blockProblem struct {
	what         string
	count        int
	line, column int
}

// blockProblems reports every kind of character in text that a YAML literal
// block scalar cannot carry as written, in the order each kind first appears:
// a carriage return, which a YAML reader reads back as a line feed; the
// characters a YAML reader takes for a line break, which would end the block
// mid-text (U+0085, U+2028, U+2029); the control characters a YAML document may
// not hold at all (U+0000 to U+001F other than the tab and the line feed,
// U+007F, and U+0080 to U+009F); the two noncharacters U+FFFE and U+FFFF; and a
// byte that is not part of a UTF-8 sequence. Every other character, a tab and a
// byte-order mark included, reads back unchanged.
func blockProblems(text string) []blockProblem {
	var order []string
	found := map[string]*blockProblem{}
	line, column := 1, 1
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		what := ""
		switch {
		case r == utf8.RuneError && size <= 1:
			what = "byte that is not UTF-8"
		case r == '\r':
			what = "carriage return"
		case r == 0x85 || r == 0x2028 || r == 0x2029:
			what = fmt.Sprintf("line-break character U+%04X", r)
		case (r < 0x20 && r != '\t' && r != '\n') || r == 0x7F || (r >= 0x80 && r <= 0x9F):
			what = fmt.Sprintf("control character U+%04X", r)
		case r == 0xFFFE || r == 0xFFFF:
			what = fmt.Sprintf("noncharacter U+%04X", r)
		}
		if what != "" {
			p, seen := found[what]
			if !seen {
				p = &blockProblem{what: what, line: line, column: column}
				found[what] = p
				order = append(order, what)
			}
			p.count++
		}
		if r == '\n' {
			line, column = line+1, 1
		} else {
			column++
		}
		i += size
	}
	var problems []blockProblem
	for _, what := range order {
		problems = append(problems, *found[what])
	}
	return problems
}

// Credential is one value a source holds in a field its format marks as a
// credential: the node holding the field, the field's name as the format names
// it, and the JSON Pointer (RFC 6901) addressing the value from the root of the
// source's tree.
type Credential struct {
	Node    string
	Field   string
	Pointer string
}

// Pointer writes the JSON Pointer (RFC 6901) whose reference tokens are tokens,
// in order: each escaped, "~" as "~0" and "/" as "~1", and each preceded by "/".
// An array element is addressed by its position written in decimal.
func Pointer(tokens ...string) string {
	var b strings.Builder
	for _, t := range tokens {
		b.WriteString("/")
		b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(t))
	}
	return b.String()
}

// pointerTokens splits a JSON Pointer into its unescaped reference tokens.
func pointerTokens(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("a JSON Pointer opens with a slash: %q", pointer)
	}
	parts := strings.Split(pointer[1:], "/")
	for i, p := range parts {
		parts[i] = strings.NewReplacer("~1", "/", "~0", "~").Replace(p)
	}
	return parts, nil
}

// scrubCredentials writes source with the value of every credential it holds
// written empty: each value a credential's pointer addresses in the source's
// bytes — each repetition of a member the source writes twice included — is
// replaced by "", unless it is already "" or null, which is left as written.
// Every other byte of the source is kept as it is. It returns the written bytes
// and the credentials whose value was emptied, in the order given.
//
// It locates the values in the source's own bytes, read as JSON, because a
// source is carried as written rather than as the tree its parser returns. A
// credential whose pointer reaches no value in the bytes is refused with an
// error, so a source whose bytes cannot be held to its tree is never carried.
func scrubCredentials(source []byte, credentials []Credential) ([]byte, []Credential, error) {
	type edit struct{ start, end int }
	var edits []edit
	var emptied []Credential
	for _, c := range credentials {
		spans, err := valueSpans(source, c.Pointer)
		if err != nil {
			return nil, nil, fmt.Errorf("the field %s of node %s: %w", c.Field, c.Node, err)
		}
		if len(spans) == 0 {
			return nil, nil, fmt.Errorf("the field %s of node %s is not in the source's bytes where its tree holds it",
				c.Field, c.Node)
		}
		changed := false
		for _, s := range spans {
			if v := string(source[s.start:s.end]); v == `""` || v == "null" {
				continue
			}
			edits = append(edits, edit(s))
			changed = true
		}
		if changed {
			emptied = append(emptied, c)
		}
	}
	if len(edits) == 0 {
		return source, nil, nil
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out []byte
	at := 0
	for _, e := range edits {
		if e.start < at {
			// A value inside a value already emptied is gone with it.
			continue
		}
		out = append(out, source[at:e.start]...)
		out = append(out, `""`...)
		at = e.end
	}
	out = append(out, source[at:]...)
	return out, emptied, nil
}

// span is the byte range of one JSON value in a document.
type span struct{ start, end int }

// errNotJSON is the refusal of a document whose bytes do not read as JSON.
var errNotJSON = errors.New("the source's bytes do not read as JSON")

// byteOrderMark is the UTF-8 encoding of U+FEFF, which a document may open with.
var byteOrderMark = []byte{0xEF, 0xBB, 0xBF}

// valueSpans returns the byte span of every value the JSON Pointer addresses in
// the JSON document doc: one per member along the way a key names, so a member
// an object writes twice yields a span for each. A document opening with a
// byte-order mark is read from after it.
func valueSpans(doc []byte, pointer string) ([]span, error) {
	tokens, err := pointerTokens(pointer)
	if err != nil {
		return nil, err
	}
	s := jsonScan{doc: doc}
	i := 0
	if bytes.HasPrefix(doc, byteOrderMark) {
		i = len(byteOrderMark)
	}
	end, err := s.find(s.space(i), tokens)
	if err != nil {
		return nil, err
	}
	if s.space(end) != len(doc) {
		return nil, errNotJSON
	}
	return s.spans, nil
}

// jsonScan reads the bytes of a JSON document for the spans of the values a
// pointer addresses.
type jsonScan struct {
	doc   []byte
	spans []span
}

func (s *jsonScan) space(i int) int {
	for i < len(s.doc) {
		switch s.doc[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

// find reads the value starting at i, recording the span of every value the
// remaining tokens address inside it, and returns the position after it.
func (s *jsonScan) find(i int, tokens []string) (int, error) {
	if len(tokens) == 0 {
		end, err := s.skip(i)
		if err != nil {
			return 0, err
		}
		s.spans = append(s.spans, span{i, end})
		return end, nil
	}
	if i >= len(s.doc) {
		return 0, errNotJSON
	}
	switch s.doc[i] {
	case '{':
		return s.object(i, tokens)
	case '[':
		return s.array(i, tokens)
	default:
		return s.skip(i)
	}
}

// object reads the object at i, descending into each member the first token
// names.
func (s *jsonScan) object(i int, tokens []string) (int, error) {
	i = s.space(i + 1)
	if i < len(s.doc) && s.doc[i] == '}' {
		return i + 1, nil
	}
	for {
		keyEnd, err := s.skipString(i)
		if err != nil {
			return 0, err
		}
		var key string
		if err := json.Unmarshal(s.doc[i:keyEnd], &key); err != nil {
			return 0, errNotJSON
		}
		i = s.space(keyEnd)
		if i >= len(s.doc) || s.doc[i] != ':' {
			return 0, errNotJSON
		}
		i = s.space(i + 1)
		if key == tokens[0] {
			i, err = s.find(i, tokens[1:])
		} else {
			i, err = s.skip(i)
		}
		if err != nil {
			return 0, err
		}
		i = s.space(i)
		if i >= len(s.doc) {
			return 0, errNotJSON
		}
		switch s.doc[i] {
		case ',':
			i = s.space(i + 1)
		case '}':
			return i + 1, nil
		default:
			return 0, errNotJSON
		}
	}
}

// array reads the array at i, descending into the element whose position the
// first token writes.
func (s *jsonScan) array(i int, tokens []string) (int, error) {
	i = s.space(i + 1)
	if i < len(s.doc) && s.doc[i] == ']' {
		return i + 1, nil
	}
	for index := 0; ; index++ {
		var err error
		if tokens[0] == strconv.Itoa(index) {
			i, err = s.find(i, tokens[1:])
		} else {
			i, err = s.skip(i)
		}
		if err != nil {
			return 0, err
		}
		i = s.space(i)
		if i >= len(s.doc) {
			return 0, errNotJSON
		}
		switch s.doc[i] {
		case ',':
			i = s.space(i + 1)
		case ']':
			return i + 1, nil
		default:
			return 0, errNotJSON
		}
	}
}

// skip returns the position after the value at i.
func (s *jsonScan) skip(i int) (int, error) {
	if i >= len(s.doc) {
		return 0, errNotJSON
	}
	switch c := s.doc[i]; {
	case c == '"':
		return s.skipString(i)
	case c == '{' || c == '[':
		close := byte('}')
		if c == '[' {
			close = ']'
		}
		i = s.space(i + 1)
		if i < len(s.doc) && s.doc[i] == close {
			return i + 1, nil
		}
		for {
			var err error
			if c == '{' {
				if i, err = s.skipString(i); err != nil {
					return 0, err
				}
				i = s.space(i)
				if i >= len(s.doc) || s.doc[i] != ':' {
					return 0, errNotJSON
				}
				i = s.space(i + 1)
			}
			if i, err = s.skip(i); err != nil {
				return 0, err
			}
			i = s.space(i)
			if i >= len(s.doc) {
				return 0, errNotJSON
			}
			switch s.doc[i] {
			case ',':
				i = s.space(i + 1)
			case close:
				return i + 1, nil
			default:
				return 0, errNotJSON
			}
		}
	default:
		start := i
		for i < len(s.doc) && strings.IndexByte("+-0123456789.eEtrufalsn", s.doc[i]) >= 0 {
			i++
		}
		if i == start {
			return 0, errNotJSON
		}
		return i, nil
	}
}

// skipString returns the position after the string at i.
func (s *jsonScan) skipString(i int) (int, error) {
	if i >= len(s.doc) || s.doc[i] != '"' {
		return 0, errNotJSON
	}
	for i++; i < len(s.doc); i++ {
		switch s.doc[i] {
		case '\\':
			i++
		case '"':
			return i + 1, nil
		}
	}
	return 0, errNotJSON
}

// flowIDNamespace is the namespace every identifier derived for a flow is a
// name-based UUID in: itself the name-based UUID of
// https://aggrega.ai/import/flow in the URL namespace RFC 9562 defines.
var flowIDNamespace = nameUUID([16]byte{0x6b, 0xa7, 0xb8, 0x11, 0x9d, 0xad, 0x11, 0xd1,
	0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}, []byte("https://aggrega.ai/import/flow"))

// derivedFlowID is the identifier a flow is given where its source names none
// the draft can carry: the name-based UUID (RFC 9562, version 5) of the
// format's name, a line feed, and name, in flowIDNamespace. One source of one
// format always yields one identifier.
func derivedFlowID(format string, name []byte) string {
	u := nameUUID(flowIDNamespace, append([]byte(format+"\n"), name...))
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// nameUUID is the version 5 UUID of name in namespace.
func nameUUID(namespace [16]byte, name []byte) [16]byte {
	h := sha1.New()
	h.Write(namespace[:])
	h.Write(name)
	var u [16]byte
	copy(u[:], h.Sum(nil))
	u[6] = (u[6] & 0x0f) | 0x50
	u[8] = (u[8] & 0x3f) | 0x80
	return u
}

// carriedIdentifier reports whether id can be written into a drafted Aggregate
// as it is: one to 128 letters, digits, dots, hyphens, underscores or colons,
// opening with a letter or a digit — the shape a flow platform's identifiers
// take, which needs no quoting and no escape.
func carriedIdentifier(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if !alnum && (i == 0 || strings.IndexByte("._:-", c) < 0) {
			return false
		}
	}
	return true
}
