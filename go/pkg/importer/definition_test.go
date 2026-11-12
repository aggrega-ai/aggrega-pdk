// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"text/template"
)

// TestEscapeTemplateTextReadsBackThroughTheTemplateEngine writes texts holding
// double braces in every arrangement a flow writes them — a variable, three
// braces, braces at the very end, a closing pair alone — and evaluates each
// escaped text as a Go template, as an Aggregate's text is evaluated before it is
// read: each reads back as the text it was. A text holding no "{{" is written
// unchanged.
//
// 🔴 PLANTED DEFECT: make escapeTemplateText return its text unchanged and every
// row holding "{{" fails, the template engine refusing the text or eating the
// braces.
func TestEscapeTemplateTextReadsBackThroughTheTemplateEngine(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		`{"prompt":"{{question}}"}`,
		`{"x":"{{chatOpenAI_0.data.instance}}","y":"{{ $json.body }}"}`,
		`{{{language}}}`,
		`ends with {{`,
		`a closing pair alone }} stays`,
		`{{"{{"}} already escaped`,
		`no braces at all`,
	} {
		t.Run(text, func(t *testing.T) {
			escaped := escapeTemplateText(text)
			if !strings.Contains(text, "{{") && escaped != text {
				t.Errorf("a text holding no double brace was rewritten as %q", escaped)
			}
			tmpl, err := template.New("aggregate").Parse(escaped)
			if err != nil {
				t.Fatalf("the escaped text %q does not parse as a template: %v", escaped, err)
			}
			var b strings.Builder
			if err := tmpl.Execute(&b, nil); err != nil {
				t.Fatalf("the escaped text %q does not run as a template: %v", escaped, err)
			}
			if b.String() != text {
				t.Errorf("the escaped text reads back as %q, want %q", b.String(), text)
			}
		})
	}
}

// TestLiteralBlockWritesTheChompingTheTextsEndNeeds pins the block header and
// the indented body for each shape a source ends or opens in: no final line
// break (strip), one (clip), several (keep), a leading space or line break (an
// indentation indicator), an empty line inside, a tab, and a key indented deeper.
func TestLiteralBlockWritesTheChompingTheTextsEndNeeds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, text string
		indent     int
		want       string
	}{
		{name: "one final line break", text: "a\nb\n", want: "|\n  a\n  b\n"},
		{name: "no final line break", text: "a\nb", want: "|-\n  a\n  b\n"},
		{name: "two final line breaks", text: "a\n\n", want: "|+\n  a\n\n"},
		{name: "a leading space", text: " a\n", want: "|2\n   a\n"},
		{name: "a leading line break", text: "\na\n", want: "|2\n\n  a\n"},
		{name: "an empty line inside", text: "a\n\nb\n", want: "|\n  a\n\n  b\n"},
		{name: "a tab", text: "a\tb\n", want: "|\n  a\tb\n"},
		{name: "a key indented eight spaces", text: "x", indent: 8, want: "|-\n          x\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := literalBlock(tc.text, tc.indent); got != tc.want {
				t.Errorf("literalBlock(%q, %d) = %q, want %q", tc.text, tc.indent, got, tc.want)
			}
		})
	}
}

// TestBlockProblemsNamesWhatALiteralBlockCannotCarry reads, for each character a
// YAML literal block cannot carry as written, the kind reported, how often and
// where it first appears; and for the characters it does carry — a tab, a
// byte-order mark, an astral character, a replacement character — nothing.
func TestBlockProblemsNamesWhatALiteralBlockCannotCarry(t *testing.T) {
	t.Parallel()
	r := func(code int) string { return string(rune(code)) }
	for _, tc := range []struct {
		name, text string
		want       []blockProblem
	}{
		{name: "what a block carries", text: "ok\tline\n" + r(0xFEFF) + r(0x1F600) + r(0xFFFD) + "\n"},
		{name: "carriage returns", text: "{\r\n  \"a\": 1\r\n}\r\n",
			want: []blockProblem{{what: "carriage return", count: 3, line: 1, column: 2}}},
		{name: "a next-line character", text: "ab\nc" + r(0x85),
			want: []blockProblem{{what: "line-break character U+0085", count: 1, line: 2, column: 2}}},
		{name: "a line and a paragraph separator", text: r(0x2028) + "x" + r(0x2029),
			want: []blockProblem{
				{what: "line-break character U+2028", count: 1, line: 1, column: 1},
				{what: "line-break character U+2029", count: 1, line: 1, column: 3},
			}},
		{name: "control characters", text: "a" + r(0x7F) + r(0x00) + r(0x90) + r(0x0B),
			want: []blockProblem{
				{what: "control character U+007F", count: 1, line: 1, column: 2},
				{what: "control character U+0000", count: 1, line: 1, column: 3},
				{what: "control character U+0090", count: 1, line: 1, column: 4},
				{what: "control character U+000B", count: 1, line: 1, column: 5},
			}},
		{name: "a noncharacter", text: "x" + r(0xFFFE),
			want: []blockProblem{{what: "noncharacter U+FFFE", count: 1, line: 1, column: 2}}},
		{name: "bytes that are not UTF-8", text: "a" + string([]byte{0xff}) + "b" + string([]byte{0xc3}),
			want: []blockProblem{{what: "byte that is not UTF-8", count: 2, line: 1, column: 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := blockProblems(tc.text); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("blockProblems reported %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestPointerEscapesEachToken pins the JSON Pointer a list of tokens is written
// as, and that pointerTokens reads it back.
func TestPointerEscapesEachToken(t *testing.T) {
	t.Parallel()
	tokens := []string{"data", "a/b", "~x", "0", ""}
	if got, want := Pointer(tokens...), "/data/a~1b/~0x/0/"; got != want {
		t.Errorf("Pointer(%q) = %q, want %q", tokens, got, want)
	}
	back, err := pointerTokens(Pointer(tokens...))
	if err != nil || !reflect.DeepEqual(back, tokens) {
		t.Errorf("pointerTokens read back %q (%v), want %q", back, err, tokens)
	}
	if _, err := pointerTokens("data/x"); err == nil {
		t.Error("a pointer not opening with a slash was read")
	}
}

// TestScrubCredentialsEmptiesOnlyTheValuesItLocates drives the scrub over a
// document written with irregular spacing: each located value is written "",
// every other byte is kept, a member the document writes twice is emptied at both
// places, a value already "" or null is left as written and not reported, a value
// that is not a string is emptied too, a value nested inside one already emptied
// goes with it, and a key spelled with an escape is matched by what it spells.
// A pointer reaching no value, and bytes that are not JSON, are refused.
func TestScrubCredentialsEmptiesOnlyTheValuesItLocates(t *testing.T) {
	t.Parallel()
	doc := "{ \"nodes\" : [ {\"id\":\"a\", \"key\" : \"sk-1\" , \"key\":\"sk-2\", \"empty\":\"\", \"none\": null}," +
		"\n  {\"id\":\"b\",\"n\":42,\"obj\":{\"inner\":\"sk-3\"},\"pass\\u0077ord\":\"sk-4\"} ] }\n"
	creds := []Credential{
		{Node: "a", Field: "key", Pointer: Pointer("nodes", "0", "key")},
		{Node: "a", Field: "empty", Pointer: Pointer("nodes", "0", "empty")},
		{Node: "a", Field: "none", Pointer: Pointer("nodes", "0", "none")},
		{Node: "b", Field: "n", Pointer: Pointer("nodes", "1", "n")},
		{Node: "b", Field: "obj", Pointer: Pointer("nodes", "1", "obj")},
		{Node: "b", Field: "obj.inner", Pointer: Pointer("nodes", "1", "obj", "inner")},
		{Node: "b", Field: "password", Pointer: Pointer("nodes", "1", "password")},
	}
	got, emptied, err := scrubCredentials([]byte(doc), creds)
	if err != nil {
		t.Fatalf("the scrub refused the document: %v", err)
	}
	want := "{ \"nodes\" : [ {\"id\":\"a\", \"key\" : \"\" , \"key\":\"\", \"empty\":\"\", \"none\": null}," +
		"\n  {\"id\":\"b\",\"n\":\"\",\"obj\":\"\",\"pass\\u0077ord\":\"\"} ] }\n"
	if string(got) != want {
		t.Errorf("the scrubbed document reads\n%s\nwant\n%s", got, want)
	}
	var fields []string
	for _, c := range emptied {
		fields = append(fields, c.Node+"/"+c.Field)
	}
	if wantFields := []string{"a/key", "b/n", "b/obj", "b/obj.inner", "b/password"}; !reflect.DeepEqual(fields, wantFields) {
		t.Errorf("the scrub reports emptying %v, want %v", fields, wantFields)
	}

	untouched, none, err := scrubCredentials([]byte(doc), nil)
	if err != nil || string(untouched) != doc || len(none) != 0 {
		t.Errorf("a scrub locating nothing changed the document or reported %v (%v)", none, err)
	}
	for _, tc := range []struct{ name, doc, pointer string }{
		{name: "a pointer reaching no value", doc: doc, pointer: Pointer("nodes", "2", "key")},
		{name: "bytes that are not JSON", doc: "{\"nodes\": [", pointer: Pointer("nodes", "0", "key")},
		{name: "bytes after the document", doc: "{\"key\":\"sk\"} trailing", pointer: Pointer("key")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := scrubCredentials([]byte(tc.doc), []Credential{{Node: "a", Field: "key", Pointer: tc.pointer}}); err == nil {
				t.Error("the scrub accepted it")
			}
		})
	}
}

// TestValueSpansAreReadThroughTheDecoder reads, for each shape a pointer meets in
// a document, the exact byte spans valueSpans returns — every span checked
// against the bytes it covers: a member reached through objects and an array
// element, a member the document writes twice (a span for each), a key spelled
// with an escape, a document opening with a byte-order mark, irregular spacing
// around every token, a number and a literal as the addressed value, and a token
// reaching into a value that is neither an object nor an array (no span). A
// pointer reaching nothing yields no span; bytes that are not one JSON document
// are refused.
//
// 🔴 PLANTED DEFECT: in collectSpans (definition.go), replace
// `if name == tokens[0] {` with `if name == tokens[0] && len(*spans) == 0 {`:
// the row of the member written twice fails, its second span lost.
func TestValueSpansAreReadThroughTheDecoder(t *testing.T) {
	t.Parallel()
	bom := string(byteOrderMark)
	for _, tc := range []struct {
		name, doc, pointer string
		want               []string
	}{
		{name: "through objects and an array", doc: `{"a":{"list":[1,{"k":"v1"},3]}}`,
			pointer: Pointer("a", "list", "1", "k"), want: []string{`"v1"`}},
		{name: "a member written twice", doc: `{"k":"one" , "x":0, "k" : "two"}`, pointer: Pointer("k"),
			want: []string{`"one"`, `"two"`}},
		{name: "a key spelled with an escape", doc: `{"password":"sk"}`, pointer: Pointer("password"),
			want: []string{`"sk"`}},
		{name: "a byte-order mark", doc: bom + `{"k":{"inner":true}}`, pointer: Pointer("k"),
			want: []string{`{"inner":true}`}},
		{name: "irregular spacing", doc: " \n{ \"n\" :\t[ 0 ,\r\n 42.5e1 ] }\n", pointer: Pointer("n", "1"),
			want: []string{`42.5e1`}},
		{name: "a literal", doc: `{"n":null}`, pointer: Pointer("n"), want: []string{`null`}},
		{name: "the whole document", doc: "  [1]  ", pointer: "", want: []string{`[1]`}},
		{name: "into a string", doc: `{"s":"text"}`, pointer: Pointer("s", "x")},
		{name: "a pointer reaching nothing", doc: `{"s":"text"}`, pointer: Pointer("t")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans, err := valueSpans([]byte(tc.doc), tc.pointer)
			if err != nil {
				t.Fatalf("the document was refused: %v", err)
			}
			var got []string
			for _, s := range spans {
				got = append(got, tc.doc[s.start:s.end])
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("valueSpans covered %q, want %q", got, tc.want)
			}
		})
	}
	for _, doc := range []string{`{"k":`, `{"k":1} {"k":2}`, `{"k":1} trailing`, bom + bom + `{}`, ``} {
		if _, err := valueSpans([]byte(doc), Pointer("k")); err == nil {
			t.Errorf("the bytes %q were read as one JSON document", doc)
		}
	}
}

// TestRewriteWritesEachEditAndKeepsEveryOtherByte rewrites a document written
// with irregular spacing: each edit's value replaces every value its pointer
// addresses, the edit given first wins where two address one value, an edit
// inside a value another replaces goes with it, and every other byte is kept. An
// edit whose value is not JSON, and one reaching nothing, are refused.
func TestRewriteWritesEachEditAndKeepsEveryOtherByte(t *testing.T) {
	t.Parallel()
	doc := "{ \"url\" : \"http://planted.example\",\n  \"k\":\"a\", \"k\":\"b\", \"obj\":{\"in\":1}, \"keep\" : [1, 2] }"
	got, err := Rewrite([]byte(doc), []Edit{
		{Pointer: Pointer("url"), Value: json.RawMessage(`""`)},
		{Pointer: Pointer("k"), Value: json.RawMessage(`"NAME"`)},
		{Pointer: Pointer("k"), Value: json.RawMessage(`"LOST"`)},
		{Pointer: Pointer("obj"), Value: json.RawMessage(`true`)},
		{Pointer: Pointer("obj", "in"), Value: json.RawMessage(`2`)},
	})
	if err != nil {
		t.Fatalf("the rewrite was refused: %v", err)
	}
	want := "{ \"url\" : \"\",\n  \"k\":\"NAME\", \"k\":\"NAME\", \"obj\":true, \"keep\" : [1, 2] }"
	if string(got) != want {
		t.Errorf("the rewrite wrote\n%s\nwant\n%s", got, want)
	}
	if same, err := Rewrite([]byte(doc), nil); err != nil || string(same) != doc {
		t.Errorf("a rewrite of no edit changed the document (%v)", err)
	}
	for _, e := range []Edit{
		{Pointer: Pointer("url"), Value: json.RawMessage(`not json`)},
		{Pointer: Pointer("absent"), Value: json.RawMessage(`""`)},
	} {
		if _, err := Rewrite([]byte(doc), []Edit{e}); err == nil {
			t.Errorf("the edit %+v was written", e)
		}
	}
}

// TestLineFeedsForWritesEachCarriageReturnAsALineFeed reads how many carriage
// returns lineFeedsFor writes and what it writes them as: a CR LF pair as one
// line feed, a lone CR as one, and a text holding none unchanged.
func TestLineFeedsForWritesEachCarriageReturnAsALineFeed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		text, want string
		count      int
	}{
		{text: "a\r\nb\r\n", want: "a\nb\n", count: 2},
		{text: "a\rb", want: "a\nb", count: 1},
		{text: "a\r\r\nb", want: "a\n\nb", count: 2},
		{text: "a\nb", want: "a\nb", count: 0},
	} {
		if got, count := lineFeedsFor(tc.text); got != tc.want || count != tc.count {
			t.Errorf("lineFeedsFor(%q) = %q, %d; want %q, %d", tc.text, got, count, tc.want, tc.count)
		}
	}
}

// uuidShape is the shape of a UUID whose version is 1 to 8 and whose variant is
// the one RFC 9562 defines — the shape a platform asks a flow identifier to take.
var uuidShape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// TestDerivedFlowIDIsTheVersion5UUIDOfTheFormatAndTheBytes pins the identifiers
// derived for a flow against values computed by an independent implementation of
// RFC 9562 version 5 — Python's uuid module:
//
//	ns = uuid.uuid5(uuid.NAMESPACE_URL, "https://aggrega.ai/import/flow")
//	uuid.uuid5(ns, b"testflow\nabc")
//
// so a change to the namespace, the name or the bit setting fails here rather than
// agreeing with itself.
func TestDerivedFlowIDIsTheVersion5UUIDOfTheFormatAndTheBytes(t *testing.T) {
	t.Parallel()
	ns := flowIDNamespace
	if got, want := strings.Join([]string{hexOf(ns[0:4]), hexOf(ns[4:6]), hexOf(ns[6:8]), hexOf(ns[8:10]), hexOf(ns[10:16])}, "-"),
		"5e142d13-046b-5479-a2aa-c4ecf56deddc"; got != want {
		t.Errorf("the namespace is %s, want %s", got, want)
	}
	for _, tc := range []struct{ format, name, want string }{
		{"testflow", "abc", "143c688e-a1dd-5cec-90b0-af86c5b5b9e6"},
		{"testflow", "a", "de39f8d6-d0f7-5021-ae19-9cef6ab1e97d"},
	} {
		got := derivedFlowID(tc.format, []byte(tc.name))
		if got != tc.want {
			t.Errorf("derivedFlowID(%q, %q) = %s, want %s", tc.format, tc.name, got, tc.want)
		}
		if !uuidShape.MatchString(got) {
			t.Errorf("%s is not of the shape a platform takes", got)
		}
	}
}

func hexOf(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, 2*len(b))
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&0x0f])
	}
	return string(out)
}

// TestCarriedIdentifierAdmitsOnlyWhatADraftWritesAsItIs reads which identifiers a
// draft writes as they are.
func TestCarriedIdentifierAdmitsOnlyWhatADraftWritesAsItIs(t *testing.T) {
	t.Parallel()
	for id, want := range map[string]bool{
		"7eb7c081-888c-4f25-ba81-c84e6aab2778": true,
		"flow-7":                               true,
		"a.b_c:d":                              true,
		"":                                     false,
		"-leading":                             false,
		"has space":                            false,
		"quote'":                               false,
		"{{x}}":                                false,
		strings.Repeat("a", 129):               false,
	} {
		if got := carriedIdentifier(id); got != want {
			t.Errorf("carriedIdentifier(%q) = %v, want %v", id, got, want)
		}
	}
}
