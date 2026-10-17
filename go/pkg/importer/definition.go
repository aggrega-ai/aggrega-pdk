// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This file holds the writer of a flow definition into a drafted Aggregate: the
// emptying of the credential values a source holds and the rewriting of the
// values a format writes anew, the escape of the text an Aggregate would
// otherwise read as a template action, the YAML literal block the definition is
// written as, and the identifier a flow is given where its source names none.

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

// lineFeedsFor writes every carriage return in text — the CR of a CR LF pair,
// and a CR standing alone — as one line feed, and returns the text and how many
// carriage returns it wrote. A YAML literal block cannot carry a carriage return
// as written. In a JSON document one stands only between tokens, where any
// whitespace reads alike, so a JSON reader reads the written text as the same
// value.
func lineFeedsFor(text string) (string, int) {
	count := strings.Count(text, "\r")
	if count == 0 {
		return text, 0
	}
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), count
}

// Credential is one value a source holds in a field its format marks as a
// credential: the node holding the field, the field's name as the format names
// it, and the JSON Pointer (RFC 6901) addressing the value from the root of the
// source's tree.
type Credential struct {
	Node    string
	Field   string
	Pointer string
	// Value, where set, is the JSON value written in place of the value instead
	// of an empty string: the same value with only the credentials it carries
	// written empty, for a field that keeps them inside text of its own, such as a
	// server configuration written as JSON text. A value that already reads as
	// Value is left as written.
	Value json.RawMessage
	// Detail, where set, says what of the value is written empty — with Value set
	// — or why the whole value is — without it. The import's report of the value
	// carries it.
	Detail string
}

// Edit is one value of a JSON document written anew: Value, itself a JSON
// value, is written in place of every value the JSON Pointer (RFC 6901) Pointer
// addresses, each repetition of a member the document writes twice included.
type Edit struct {
	Pointer string
	Value   json.RawMessage
}

// Rewrite writes each edit into the JSON document doc and returns the document
// with every other byte kept as it is. It locates the values through the JSON
// decoder the package reads every tree with, so it reads doc as that decoder
// does; a document opening with a byte-order mark is read from after it.
//
// It refuses a pointer that reaches no value in doc, a Value that is not JSON,
// and a doc that is not one JSON document. Where two edits address one value,
// the one given first is written; an edit addressing a value inside one another
// edit replaces goes with it.
func Rewrite(doc []byte, edits []Edit) ([]byte, error) {
	out, _, err := rewriteSource(doc, edits, nil)
	return out, err
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
	return rewriteSource(source, nil, credentials)
}

// rewriteSource writes each edit into source and then empties every credential
// as scrubCredentials does: a credential carrying a Value has it written in
// place of its value instead of "", and is left as written where its value
// already reads as that. An edit wins over a credential addressing the same
// value, and the credential is then not reported as emptied, since what is
// written there is the edit's. Every other byte is kept. It returns the written
// bytes and the credentials whose value it changed, in the order given.
//
// An edit or a credential whose pointer reaches no value in the bytes is refused
// with an error, as are an edit whose value is not JSON and bytes that are not
// one JSON document.
func rewriteSource(source []byte, edits []Edit, credentials []Credential) ([]byte, []Credential, error) {
	type write struct {
		span
		value []byte
		// order is the write's position among the edits, which come first, and
		// then the credentials: where two writes address one value, the earlier
		// one is written.
		order int
	}
	var writes []write
	edited := map[span]bool{}
	for i, e := range edits {
		if !json.Valid(e.Value) {
			return nil, nil, fmt.Errorf("the value written at %s is not JSON", e.Pointer)
		}
		spans, err := valueSpans(source, e.Pointer)
		if err != nil {
			return nil, nil, fmt.Errorf("the value at %s: %w", e.Pointer, err)
		}
		if len(spans) == 0 {
			return nil, nil, fmt.Errorf("the value at %s is not in the source's bytes where its tree holds it", e.Pointer)
		}
		for _, s := range spans {
			writes = append(writes, write{span: s, value: e.Value, order: i})
			edited[s] = true
		}
	}
	var changed []Credential
	for i, c := range credentials {
		spans, err := valueSpans(source, c.Pointer)
		if err != nil {
			return nil, nil, fmt.Errorf("the field %s of node %s: %w", c.Field, c.Node, err)
		}
		if len(spans) == 0 {
			return nil, nil, fmt.Errorf("the field %s of node %s is not in the source's bytes where its tree holds it",
				c.Field, c.Node)
		}
		value := []byte(`""`)
		if c.Value != nil {
			if !json.Valid(c.Value) {
				return nil, nil, fmt.Errorf("the value written in the field %s of node %s is not JSON", c.Field, c.Node)
			}
			value = c.Value
		}
		wrote := false
		for _, s := range spans {
			if edited[s] {
				continue
			}
			v := source[s.start:s.end]
			if bytes.Equal(v, value) || (c.Value == nil && string(v) == "null") {
				continue
			}
			writes = append(writes, write{span: s, value: value, order: len(edits) + i})
			wrote = true
		}
		if wrote {
			changed = append(changed, c)
		}
	}
	if len(writes) == 0 {
		return source, nil, nil
	}
	sort.SliceStable(writes, func(i, j int) bool {
		if writes[i].start != writes[j].start {
			return writes[i].start < writes[j].start
		}
		return writes[i].order < writes[j].order
	})
	var out []byte
	at := 0
	for _, w := range writes {
		if w.start < at {
			// A value inside one already written, or one written already, goes
			// with the write that replaced it.
			continue
		}
		out = append(out, source[at:w.start]...)
		out = append(out, w.value...)
		at = w.end
	}
	out = append(out, source[at:]...)
	return out, changed, nil
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
//
// It reads doc through encoding/json's Decoder, the reader every tree is read
// with here, so the package carries no JSON grammar of its own: a value's bytes
// are decoded as they stand, its end is the decoder's input offset once it is
// read and its start that end less its length, and a container the pointer
// descends into is read the same way from its own bytes, its offset carried.
func valueSpans(doc []byte, pointer string) ([]span, error) {
	tokens, err := pointerTokens(pointer)
	if err != nil {
		return nil, err
	}
	base := 0
	if bytes.HasPrefix(doc, byteOrderMark) {
		base = len(byteOrderMark)
	}
	dec := json.NewDecoder(bytes.NewReader(doc[base:]))
	var whole json.RawMessage
	if err := dec.Decode(&whole); err != nil {
		return nil, errNotJSON
	}
	end := base + int(dec.InputOffset())
	if _, err := dec.Token(); err != io.EOF {
		return nil, errNotJSON
	}
	var spans []span
	collectSpans(whole, end-len(whole), tokens, &spans)
	return spans, nil
}

// collectSpans records the span of every value the tokens address inside raw,
// one JSON value of the document starting at the offset at. A token naming a
// member of an object is matched against each member's key as the key reads, so
// a key spelled with an escape matches what it spells; one naming an element of
// an array is its position, written in decimal. A token reaching into a value
// that is neither yields nothing.
func collectSpans(raw json.RawMessage, at int, tokens []string, spans *[]span) {
	if len(tokens) == 0 {
		*spans = append(*spans, span{at, at + len(raw)})
		return
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	open, err := dec.Token()
	if err != nil {
		return
	}
	delim, ok := open.(json.Delim)
	if !ok || (delim != '{' && delim != '[') {
		return
	}
	for index := 0; dec.More(); index++ {
		name := strconv.Itoa(index)
		if delim == '{' {
			key, err := dec.Token()
			if err != nil {
				return
			}
			name, _ = key.(string)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return
		}
		if name == tokens[0] {
			end := at + int(dec.InputOffset())
			collectSpans(value, end-len(value), tokens[1:], spans)
		}
	}
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
