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

// draftedText writes text read from a source as the value of a key whose line
// sits at keyIndent spaces in a drafted Aggregate: escaped by escapeTemplateText
// and written as a YAML literal block by literalBlock, so that once the
// Aggregate's text has been evaluated as a template and read as YAML the value
// is a string and reads back as text, whatever text holds. Every value an import
// copies from a source into a drafted Aggregate goes through it — a flow
// definition, a model's name — but a flow's own identifier, which is copied only
// where it holds nothing either step would change (carriedIdentifier), and is
// written single-quoted as it is.
//
// The text must hold only what blockProblems admits; the caller checks first.
func draftedText(text string, keyIndent int) string {
	return literalBlock(escapeTemplateText(text), keyIndent)
}

// escapeTemplateText writes text so that it reads back as itself once an
// Aggregate's text has been evaluated as a template: every "{{" is written as
// {{"{{"}}, an action whose output is the two braces, and nothing else is
// changed. A closing "}}" outside an action is plain text to the template
// engine and is left as written.
//
// It is the escape a carried flow is written with, reached through draftedText.
// The escaped text carries double quotes, so it is written where a double quote
// is an ordinary character, as a literal block scalar is.
//
// It is a deliberate near-duplicate of EscapeTemplateText, the two told apart by
// the quoting context their output is written in; do not unify them. A literal
// block holds a double quote as an ordinary character, so the flow escape keeps
// {{"{{"}}, the spelling the Aggregate's documentation teaches an author for a
// value carrying double braces of its own, and the bytes a carried flow is
// written with. EscapeTemplateText's raw-string action would read back alike
// here, but writing it would change every carried flow's bytes for no reader's
// gain.
func escapeTemplateText(text string) string {
	return strings.ReplaceAll(text, "{{", `{{"{{"}}`)
}

// EscapeTemplateText writes text read from a source so that it reads back as
// itself once a drafted Aggregate's text has been evaluated as a template: every
// "{{" is written as {{`{{`}}, an action whose output is the two braces, and
// nothing else is changed. An importer drafting an Aggregate of its own writes
// each value it copies from a source through it, so a value carrying two opening
// braces is carried as written by every importer. The action quotes the braces
// with a raw string rather than with double quotes, so the escaped text holds no
// double quote and no backslash: it can be written inside a double-quoted
// scalar, whose escapes the template engine would otherwise read first, since
// it evaluates the Aggregate's text before the text is read as YAML.
//
// It is a deliberate near-duplicate of the escape a carried flow is written
// with, the two told apart by the quoting context their output is written in;
// do not unify them. The flow escape's {{"{{"}} holds double quotes, which a
// double-quoted scalar writes as \", so the template engine would meet a
// backslash inside the action and refuse the Aggregate; the raw-string action
// holds in every scalar style.
func EscapeTemplateText(text string) string {
	return strings.ReplaceAll(text, "{{", "{{`{{`}}")
}

// CommentLine writes one comment line of a drafted Aggregate: indent, the hash
// and a space, text escaped as EscapeTemplateText escapes a value, and a line
// feed. The compiler evaluates an Aggregate's whole text as a template before
// reading it as YAML, its comments included, so a comment holding "{{" written
// as it is would halt the build; escaped, it reads back as written. It is the
// one writer of an Aggregate's comment lines, for the kit's own drafts and for
// every importer drafting an Aggregate of its own.
//
// The escape is applied to the whole line rather than to each text a source
// supplies inside it: the line's own words hold no "{{", so the two read back
// alike, and each text read from a source is written first through the drafts'
// comment writer, which keeps it on its line. A Release is not evaluated as a
// template, so its comments are written as they are.
func CommentLine(indent, text string) string {
	return indent + "# " + EscapeTemplateText(text) + "\n"
}

// QuotedString writes s as a double-quoted YAML scalar reading back as exactly s,
// whatever UTF-8 text it holds:
// a double quote and a backslash escaped, every printable ASCII character as it
// is, and every other character as its \u or \U escape, so no character a YAML
// reader could take for a line break, and no control character, is written raw.
// A byte that is not part of a UTF-8 sequence is written as U+FFFD. It is the one
// writer of a double-quoted scalar for the kit's own drafts and for every
// importer drafting YAML by hand. A value written into a drafted Aggregate, whose
// text is evaluated as a template before it is read, is written through
// EscapeTemplateText first, whose output holds no double quote and no backslash
// of its own.
func QuotedString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r >= 0x20 && r < 0x7f:
			b.WriteRune(r)
		case r <= 0xffff:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			fmt.Fprintf(&b, `\U%08X`, r)
		}
	}
	b.WriteByte('"')
	return b.String()
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
		// A byte below utf8.RuneSelf is the character it encodes, as decoding it
		// would find.
		r, size := rune(text[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(text[i:])
		}
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

// Credential is one value a source holds that can carry a credential, as a
// format's credential locator finds it: the node holding the field, the field's
// name as the format names it, the JSON Pointer (RFC 6901) addressing the value
// from the root of the source's tree, and its class — what the format says of
// the field, which decides which value of the empty_credentials setting writes
// it empty. A value the setting does not write empty is carried as written and
// named in a warning. In a flow a platform draft carries, a value that is wholly
// a variable reference, as [IsVariableReference] reads one, holds no value: it
// is left as written under every setting and named in no warning.
type Credential struct {
	Node    string
	Field   string
	Pointer string
	// Class is what the format says of the field: that it marks it as a
	// credential, the zero value; that it is a member of an MCP server's
	// configuration; or that it marks it as none though it can carry one.
	Class CredentialClass
	// Value, where set, is the JSON value written in place of the value instead
	// of an empty string where it is written empty: the same value with only the
	// credentials it carries written empty, for a field that keeps them inside
	// text of its own, such as a server configuration written as JSON text. A
	// value that already reads as Value is left as written, and holds nothing to
	// carry. The decision sees only the value as a whole, so the locator writing
	// Value keeps as written each part inside it that is wholly a variable
	// reference, as [IsVariableReference] reads one, since such a part holds no
	// value.
	Value json.RawMessage
	// Detail, where set, says what of the value is written empty — with Value set
	// — or why the whole value is — without it. The import's report of a value
	// written empty carries it, and the report of a value carried with Value set
	// names what Detail names.
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

// rewriteSource is rewriteSourceAt returning, in place of a flag per
// credential, the credentials whose value it changed, in the order given.
func rewriteSource(source []byte, edits []Edit, credentials []Credential) ([]byte, []Credential, error) {
	out, changed, err := rewriteSourceAt(source, edits, credentials)
	if err != nil {
		return nil, nil, err
	}
	var written []Credential
	for i, c := range credentials {
		if changed[i] {
			written = append(written, c)
		}
	}
	return out, written, nil
}

// rewriteSourceAt writes each edit into source and then empties every
// credential as scrubCredentials does: a credential carrying a Value has it
// written in place of its value instead of "", and is left as written where its
// value already reads as that. An edit wins over a credential addressing the
// same value, and the credential is then not reported as emptied, since what is
// written there is the edit's. Every other byte is kept. It returns the written
// bytes and, for each credential in the order given, whether it changed its
// value.
//
// Every edit and credential is located in one reading of source, as a
// jsonDocument reads it, however many there are.
//
// An edit or a credential whose pointer reaches no value in the bytes is refused
// with an error — a credentialNotLocated error for a credential — as are an edit
// whose value is not JSON and bytes that are not one JSON document.
func rewriteSourceAt(source []byte, edits []Edit, credentials []Credential) ([]byte, []bool, error) {
	type write struct {
		span
		value []byte
		// order is the write's position among the edits, which come first, and
		// then the credentials: where two writes address one value, the earlier
		// one is written.
		order int
	}
	doc := newJSONDocument(source)
	pointers := make([]string, 0, len(edits)+len(credentials))
	for _, e := range edits {
		pointers = append(pointers, e.Pointer)
	}
	for _, c := range credentials {
		pointers = append(pointers, c.Pointer)
	}
	doc.prepare(pointers)
	var writes []write
	edited := map[span]bool{}
	for i, e := range edits {
		if !json.Valid(e.Value) {
			return nil, nil, fmt.Errorf("the value written at %s is not JSON", e.Pointer)
		}
		spans, err := doc.spans(e.Pointer)
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
	changed := make([]bool, len(credentials))
	for i, c := range credentials {
		spans, err := doc.spans(c.Pointer)
		if err != nil {
			return nil, nil, credentialNotLocated{fmt.Errorf("the field %s of node %s: %w", c.Field, c.Node, err)}
		}
		if len(spans) == 0 {
			return nil, nil, credentialNotLocated{fmt.Errorf("the field %s of node %s is not in the source's bytes "+
				"where its tree holds it", c.Field, c.Node)}
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
		changed[i] = wrote
	}
	if len(writes) == 0 {
		return source, changed, nil
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
// the JSON document doc, as [jsonDocument.spans] reads them.
func valueSpans(doc []byte, pointer string) ([]span, error) {
	return newJSONDocument(doc).spans(pointer)
}

// jsonDocument is one JSON document read for the byte spans of the values JSON
// Pointers address in it. The document is read as one JSON value once, and each
// container a pointer descends into is listed once: every container on the way
// of the pointers handed to prepare in the one reading prepare makes, and any
// other when the first pointer descends into it. So the spans of every pointer
// into one document cost one reading of it, however many pointers there are.
//
// It reads through encoding/json's Decoder, the reader every tree is read with
// here, so the package carries no JSON grammar of its own: a value's bytes are
// decoded as they stand, its end is the decoder's input offset once it is read,
// and its start that end less its length — or, for a container read through its
// tokens, the offset of its opening token.
type jsonDocument struct {
	bytes []byte
	// read records that the document has been read: root is then the span of
	// its value, or err says why its bytes are not one JSON document.
	read bool
	root span
	err  error
	// listed are the members of each container listed so far, by the offset its
	// value starts at.
	listed map[int]containerMembers
}

// containerMembers are the members of one object or array, in the order the
// document writes them: each one's name — an object member's key as the key
// reads, so a key spelled with an escape reads as what it spells, or an array
// element's position written in decimal — and the span of its value.
type containerMembers struct {
	names []string
	spans []span
}

// valueLength is a decoded JSON value's length in bytes. The Decoder hands a
// value's own bytes to the value's UnmarshalJSON, so decoding into a
// valueLength measures a value without copying it.
type valueLength int

// UnmarshalJSON records the length of data, the bytes of one JSON value.
func (n *valueLength) UnmarshalJSON(data []byte) error {
	*n = valueLength(len(data))
	return nil
}

// newJSONDocument returns doc, to be read for the spans of the values pointers
// address in it.
func newJSONDocument(doc []byte) *jsonDocument {
	return &jsonDocument{bytes: doc, listed: map[int]containerMembers{}}
}

// spans returns the byte span of every value the JSON Pointer addresses in the
// document: one per member along the way a key names, so a member an object
// writes twice yields a span for each. A pointer that does not read as one is
// refused as such, whatever the document holds; bytes that are not one JSON
// document are refused for every other pointer.
func (d *jsonDocument) spans(pointer string) ([]span, error) {
	tokens, err := pointerTokens(pointer)
	if err != nil {
		return nil, err
	}
	if !d.read {
		d.root, d.err = readDocument(d.bytes)
		d.read = true
	}
	if d.err != nil {
		return nil, d.err
	}
	var spans []span
	d.collectSpans(d.root, tokens, &spans)
	return spans, nil
}

// readDocument reads doc as one JSON value and returns its span. A document
// opening with a byte-order mark is read from after it; bytes that are not one
// JSON value, and bytes after it, are refused.
func readDocument(doc []byte) (span, error) {
	base := 0
	if bytes.HasPrefix(doc, byteOrderMark) {
		base = len(byteOrderMark)
	}
	dec := json.NewDecoder(bytes.NewReader(doc[base:]))
	var n valueLength
	if err := dec.Decode(&n); err != nil {
		return span{}, errNotJSON
	}
	end := base + int(dec.InputOffset())
	if _, err := dec.Token(); err != io.EOF {
		return span{}, errNotJSON
	}
	return span{end - int(n), end}, nil
}

// pathTrie is the containers on the way of a set of JSON Pointers: under each
// member name, the containers on the way below that member.
type pathTrie map[string]pathTrie

// prepare reads the document for the pointers it will be asked about, in one
// reading: it reads the document as one JSON value, as readDocument does, and
// lists, as membersOf would, every container on the way of a pointer — reading
// a container on the way through its tokens, and so in the same reading as the
// container holding it, and decoding every other member whole. A pointer that
// does not read as one is passed over; spans refuses it when asked. Where the
// document is not one JSON value, every pointer is refused, as readDocument
// refuses it; where a value on the way is not a container, the container
// holding it is listed when a pointer descends into it, and where the document's
// value itself is not one, it is read when the first pointer is asked about.
func (d *jsonDocument) prepare(pointers []string) {
	if d.read {
		return
	}
	way, descends := pathTrie{}, false
	for _, p := range pointers {
		tokens, err := pointerTokens(p)
		if err != nil || len(tokens) == 0 {
			continue
		}
		descends = true
		node := way
		for _, t := range tokens[:len(tokens)-1] {
			next, ok := node[t]
			if !ok {
				next = pathTrie{}
				node[t] = next
			}
			node = next
		}
	}
	if !descends {
		return
	}
	base := 0
	if bytes.HasPrefix(d.bytes, byteOrderMark) {
		base = len(byteOrderMark)
	}
	dec := json.NewDecoder(bytes.NewReader(d.bytes[base:]))
	// Numbers read as tokens are kept as written, so that reading one never
	// refuses a number its own decoding accepts.
	dec.UseNumber()
	root, known, err := d.stream(dec, base, way)
	if err == nil {
		if _, end := dec.Token(); end != io.EOF {
			err = errNotJSON
		}
	}
	switch {
	case err != nil:
		d.read, d.err, d.listed = true, errNotJSON, map[int]containerMembers{}
	case known:
		d.read, d.root = true, root
	}
}

// stream reads the next value from dec, whose input starts at offset base of the
// document, and returns its span. Where way is nil the value is decoded whole.
// Otherwise it is a container on the way of a pointer: its tokens are read, each
// member is read as stream reads the member's way, and the container's members
// are listed — unless the span of one of them could not be read, in which case
// membersOf lists it when a pointer descends into it. known is false where the
// value's span could not be read: a value on the way that is not a container,
// which the decoder hands over as a token rather than as its bytes.
func (d *jsonDocument) stream(dec *json.Decoder, base int, way pathTrie) (at span, known bool, err error) {
	if way == nil {
		var n valueLength
		if err := dec.Decode(&n); err != nil {
			return span{}, false, err
		}
		end := base + int(dec.InputOffset())
		return span{end - int(n), end}, true, nil
	}
	open, err := dec.Token()
	if err != nil {
		return span{}, false, err
	}
	delim, ok := open.(json.Delim)
	if !ok {
		return span{}, false, nil
	}
	start := base + int(dec.InputOffset()) - 1
	var members containerMembers
	listed := true
	for index := 0; dec.More(); index++ {
		name := strconv.Itoa(index)
		if delim == '{' {
			key, err := dec.Token()
			if err != nil {
				return span{}, false, err
			}
			name, _ = key.(string)
		}
		member, known, err := d.stream(dec, base, way[name])
		if err != nil {
			return span{}, false, err
		}
		if !known {
			listed = false
			continue
		}
		members.names = append(members.names, name)
		members.spans = append(members.spans, member)
	}
	if _, err := dec.Token(); err != nil {
		return span{}, false, err
	}
	if listed {
		d.listed[start] = members
	}
	return span{start, base + int(dec.InputOffset())}, true, nil
}

// collectSpans records the span of every value the tokens address inside the
// value spanning at: that value itself where no token is left; otherwise every
// member of it, in turn, whose name is the first token. A token reaching into a
// value that is neither an object nor an array yields nothing.
func (d *jsonDocument) collectSpans(at span, tokens []string, spans *[]span) {
	if len(tokens) == 0 {
		*spans = append(*spans, at)
		return
	}
	members := d.membersOf(at)
	for i, name := range members.names {
		if name == tokens[0] {
			d.collectSpans(members.spans[i], tokens[1:], spans)
		}
	}
}

// membersOf returns the members of the value spanning at, listing them the
// first time a pointer descends into it: its opening token, then each member's
// key, for an object, and its value. A value that is neither an object nor an
// array has none.
func (d *jsonDocument) membersOf(at span) containerMembers {
	if members, listed := d.listed[at.start]; listed {
		return members
	}
	var members containerMembers
	dec := json.NewDecoder(bytes.NewReader(d.bytes[at.start:at.end]))
	open, err := dec.Token()
	delim, _ := open.(json.Delim)
	if err == nil && (delim == '{' || delim == '[') {
		for index := 0; dec.More(); index++ {
			name := strconv.Itoa(index)
			if delim == '{' {
				key, err := dec.Token()
				if err != nil {
					break
				}
				name, _ = key.(string)
			}
			var n valueLength
			if err := dec.Decode(&n); err != nil {
				break
			}
			end := at.start + int(dec.InputOffset())
			members.names = append(members.names, name)
			members.spans = append(members.spans, span{end - int(n), end})
		}
	}
	d.listed[at.start] = members
	return members
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
