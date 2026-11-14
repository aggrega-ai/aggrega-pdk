// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// This file holds the one decision an import makes over the credential values a
// format locates: which of them the empty_credentials setting writes empty, and
// which are carried as written, each named in a warning that never shows the
// value; and the one test of a value that is wholly a variable reference, which
// holds no value of its own.

// IsVariableReference reports whether text is one variable reference and
// nothing else, and so holds no value of its own: the bare name of a variable,
// ${NAME}, NAME being a letter or an underscore followed by letters, digits and
// underscores — a predefined variable such as ${workspaceFolder} or ${userHome}
// included — or one of the forms the MCP clients define for a server's
// configuration, ${env:NAME} and ${input:id}, the name or the id written in
// letters, digits, underscores and hyphens. Every other form holds a value: a
// reference carrying a default, an alternative, a message or an assignment —
// ${NAME:-value}, ${NAME-value}, ${NAME:+value}, ${NAME+value},
// ${NAME:?message}, ${NAME?message}, ${NAME:=value}, ${NAME=value} — holds the
// text after its operator, which can be a key; any other text inside ${…}, such
// as ${config:name} or ${/}, is not a reference here and holds that text; and
// text holding anything beside a reference holds that.
//
// A flow a platform draft carries is judged by it: a located value that is
// wholly a variable reference is left as written under every value of the
// empty_credentials setting and named in no warning. An importer drafting
// values of its own reads references in its format's grammar, this one where
// its format's is the MCP clients', and hands [CarryCredentials] only the
// values that grammar says hold one.
func IsVariableReference(text string) bool {
	if !strings.HasPrefix(text, "${") || !strings.HasSuffix(text, "}") || len(text) < 4 {
		return false
	}
	inner := text[2 : len(text)-1]
	for _, form := range []string{"env:", "input:"} {
		if strings.HasPrefix(inner, form) {
			return referenceID(inner[len(form):])
		}
	}
	return variableName(inner)
}

// variableName reports whether name is the bare name of a variable: a letter or
// an underscore, then letters, digits and underscores. Any other character ends
// the name, so a name followed by an operator and its text is not one.
func variableName(name string) bool {
	for i, r := range name {
		if !(r == '_' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || (i > 0 && '0' <= r && r <= '9')) {
			return false
		}
	}
	return name != ""
}

// referenceID reports whether id names what ${env:…} or ${input:…} reads: one
// or more letters, digits, underscores and hyphens.
func referenceID(id string) bool {
	for _, r := range id {
		if !(r == '_' || r == '-' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9')) {
			return false
		}
	}
	return id != ""
}

// CredentialClass is what a format says of a credential value its locator
// returns, and so which value of the empty_credentials setting writes it empty.
type CredentialClass int

const (
	// CredentialMarked is a value in a field the format itself marks as a
	// credential, such as a field the format declares secret or the stored
	// credential a node selects. empty_credentials set to marked or all writes it
	// empty.
	CredentialMarked CredentialClass = iota
	// CredentialServerMember is a member of an MCP server's configuration,
	// wherever a format keeps one: a header or an environment value, the
	// server's address, the command starting it or an argument it is started
	// with. No format marks one as a credential, and each can carry a key.
	// empty_credentials set to marked or all writes it empty.
	CredentialServerMember
	// CredentialUnmarked is an input the format marks as no credential and that
	// can carry one all the same: a request header, a query parameter, a request
	// written as a command line, an input named for a key. empty_credentials set
	// to all writes it empty.
	CredentialUnmarked
)

// The empty_credentials setting and the values it takes.
const (
	emptyCredentialsSetting = "empty_credentials"
	emptyNone               = "none"
	emptyMarked             = "marked"
	emptyAll                = "all"
)

// credentialRemoval is what the empty_credentials setting asks of the credential
// values an import locates: the level it names, none where it names none.
type credentialRemoval struct {
	level string
}

// readCredentialRemoval reads the empty_credentials setting off the settings
// document: none where the document does not set it, or is not a JSON object;
// the value it names where that is one of none, marked and all. Any other value
// it holds — another text, another kind of value, null — is one the setting does
// not take, and the diagnostic returned halts the import naming the setting and
// the value, since reading it as none would carry the values its author asked
// to write empty.
//
// It reads the document's own members rather than the decoded settings, so that
// a value another setting holds that does not decode cannot leave this one
// unread. Where the document sets it more than once, the last is read, as the
// document's decoding reads it.
func readCredentialRemoval(settings json.RawMessage) (credentialRemoval, *aggrega.Diagnostic) {
	var raw json.RawMessage
	for _, e := range ObjectEntries(settings) {
		if e.Key == emptyCredentialsSetting {
			raw = e.Value
		}
	}
	if raw == nil {
		return credentialRemoval{level: emptyNone}, nil
	}
	var level string
	if json.Unmarshal(raw, &level) == nil {
		switch level {
		case emptyNone, emptyMarked, emptyAll:
			return credentialRemoval{level: level}, nil
		}
	}
	return credentialRemoval{}, &aggrega.Diagnostic{
		Severity: "halt",
		Code:     "unknown-empty-credentials",
		Message: fmt.Sprintf("the %s setting is %s, which is none of %s, %s and %s, so the import drafts nothing: "+
			"an import writes a credential value empty only where the setting asks for it", emptyCredentialsSetting,
			strings.TrimSpace(string(raw)), emptyNone, emptyMarked, emptyAll),
	}
}

// empties reports whether the setting writes a value of class empty.
func (r credentialRemoval) empties(class CredentialClass) bool {
	switch r.level {
	case emptyAll:
		return true
	case emptyMarked:
		return class == CredentialMarked || class == CredentialServerMember
	}
	return false
}

// levelEmptying names the values of empty_credentials that write a value of
// class empty.
func levelEmptying(class CredentialClass) string {
	if class == CredentialUnmarked {
		return emptyAll
	}
	return emptyMarked + " or " + emptyAll
}

// CarryCredentials is the decision the empty_credentials setting makes over the
// credential values an importer locates in a JSON document it drafts, for an
// importer that drafts values of its own rather than carrying a source: the one
// decision a platform draft makes over the flow it carries, so a draft made
// either way follows one setting, under the same codes and the same messages.
//
// settings is the settings document the import is handed, doc the drafted JSON
// document, and located the values the importer's own locator finds in it, each
// addressed from the root of doc and named by the reference its Node holds and
// its field. where completes the sentence reporting each value, naming where doc
// is written, such as "in the Release the draft writes".
//
// It returns doc with each located value the setting writes empty written as
// [Credential] says — "" or its Value — and every other byte kept, and one
// diagnostic per located value it changes or carries, in the order located: a
// value written empty reported under credential-emptied; a value carried as
// written that holds something — not "", not null, and not already what its
// Value would write — under credential-carried, naming the node, the field and
// the setting value that writes it empty, and never any part of the value. A
// value the setting carries is never located in doc's bytes, so it never stops
// doc being written. It judges every located value by its class alone: the
// importer reads its own format's variable references and hands none of them
// here, as [IsVariableReference] says.
//
// Where the setting holds a value it does not take, nothing is written: the
// document returned is nil and the diagnostics hold one halting diagnostic
// naming the setting and the value, and the importer drafts nothing from doc.
// A value the setting writes empty whose pointer reaches nothing in doc is
// refused with an error, as a document whose bytes are not one JSON document is.
func CarryCredentials(settings json.RawMessage, doc []byte, where string, located []Credential) ([]byte, []aggrega.Diagnostic, error) {
	removal, halt := readCredentialRemoval(settings)
	if halt != nil {
		return nil, []aggrega.Diagnostic{*halt}, nil
	}
	out, diags, _, err := carryCredentials(removal, doc, doc, nil, nil, located, where, nil, sourceWriter{}, false)
	return out, diags, err
}

// sourceWriter is how the values an import writes empty are written into a
// source's own bytes: located there read as JSON, where empty is nil; otherwise
// written by the format's empty — [Platform.EmptyInSource] — and held to the
// source's tree through reparse, the host's parser.
type sourceWriter struct {
	empty   func(source []byte, credentials []Credential) ([]byte, error)
	reparse func(source []byte) (json.RawMessage, error)
}

// write writes each edit into source and then each credential, as
// rewriteSourceAt does, returning the written bytes and, for each credential in
// the order given, whether it changed its value. With an empty of the format's
// own, which edits nothing, it decides off tree which credentials hold a value
// to write — one neither "", nor null, nor already its Value — hands exactly
// those to empty, and keeps what empty returns only where it reads back, through
// reparse, as tree with exactly those values written: a credentialNotLocated
// error otherwise, so the source is not carried. With none to write, source is
// returned as it is and empty is not asked.
func (w sourceWriter) write(source []byte, tree json.RawMessage, edits []Edit,
	credentials []Credential) ([]byte, []bool, error) {
	if w.empty == nil {
		return rewriteSourceAt(source, edits, credentials)
	}
	if len(edits) > 0 {
		return nil, nil, errors.New("its format writes values anew only in a source read as JSON")
	}
	want, changed, err := rewriteSourceAt(tree, nil, credentials)
	if err != nil {
		return nil, nil, err
	}
	var asked []Credential
	for i, c := range credentials {
		if changed[i] {
			asked = append(asked, c)
		}
	}
	if len(asked) == 0 {
		return source, changed, nil
	}
	out, err := w.empty(source, asked)
	if err != nil {
		return nil, nil, credentialNotLocated{fmt.Errorf("its credential values could not be written empty in its own "+
			"bytes: %w", err)}
	}
	if !sameTree(w.reparse, out, want) {
		return nil, nil, credentialNotLocated{errors.New("its bytes with its credential values written empty do not " +
			"read back as its tree with exactly those values emptied")}
	}
	return out, changed, nil
}

// sameTree reports whether source, parsed through reparse, reads as the tree
// want: the same value, compared member by member and numbers by their digits,
// whatever order or spacing either writes it in. A source reparse refuses reads
// as no tree.
func sameTree(reparse func([]byte) (json.RawMessage, error), source []byte, want json.RawMessage) bool {
	if reparse == nil {
		return false
	}
	got, err := reparse(source)
	if err != nil {
		return false
	}
	a, okA := decodedTree(got)
	b, okB := decodedTree(want)
	return okA && okB && reflect.DeepEqual(a, b)
}

// readAlike reports whether a and b, each parsed through reparse, read as the
// same tree.
func readAlike(reparse func([]byte) (json.RawMessage, error), a, b []byte) bool {
	if reparse == nil {
		return false
	}
	want, err := reparse(b)
	return err == nil && sameTree(reparse, a, want)
}

// decodedTree decodes one JSON document, its numbers kept as written.
func decodedTree(raw json.RawMessage) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return v, true
}

// credentialNotLocated is the refusal of a credential value the setting writes
// empty that the source's bytes do not hold where its tree does.
type credentialNotLocated struct{ err error }

func (e credentialNotLocated) Error() string { return e.err.Error() }

func (e credentialNotLocated) Unwrap() error { return e.err }

// carryCredentials writes source as removal asks of the located credential
// values: each edit written first, as [Rewrite] writes it; each located value an
// edit writes — at its pointer, or at a value holding it — left to the edit,
// neither written empty nor carried, since the edit is what the value is written
// as; with references set, each other value that is wholly a variable reference
// in tree, as wholeReference reads one, left as written and named in no report,
// since it holds no value; each other value of a class removal empties written
// empty, located in the source's bytes; and each other value carried as written,
// judged off tree, never located in the bytes. refOf names a node in a report,
// nil naming it as located. writer writes the edits and the values to empty into
// the source's bytes, as [sourceWriter.write] does. It returns the written bytes
// and the diagnostics, as [CarryCredentials] describes them. A value to write
// empty that the bytes do not hold, or that the format's own writer could not
// write so the source reads back as its tree with exactly that value written, is
// refused with a credentialNotLocated error; an edit the bytes do not hold, or
// bytes that are not one JSON document, with another error.
//
// references is set for a flow a platform draft carries, whose values the kit
// reads in the one grammar [IsVariableReference] states, and unset for
// [CarryCredentials], whose importer has read its own format's references
// already: a format whose interpolation reads another grammar — one reading
// ${NAME-default} as its default — hands such a value here as one holding text.
//
// respelled are the edits a platform's Respell asks for, which are no wiring and
// never decide a located value. One is written, after edits, only where it
// touches no value an edit of edits writes and no located value — neither at it,
// nor holding it, nor lying inside it — so what the wiring writes stays the
// wiring's, and what can carry a credential stays removal's to decide, exactly as
// with no respelling. The returned respelling counts the edits written and those
// left out.
func carryCredentials(removal credentialRemoval, source []byte, tree json.RawMessage, edits, respelled []Edit,
	located []Credential, where string, refOf func(string) string,
	writer sourceWriter, references bool) ([]byte, []aggrega.Diagnostic, respelling, error) {
	if refOf == nil {
		refOf = func(node string) string { return node }
	}
	var editTokens [][]string
	for _, e := range edits {
		tokens, err := pointerTokens(e.Pointer)
		if err != nil {
			return nil, nil, respelling{}, fmt.Errorf("the value at %s: %w", e.Pointer, err)
		}
		editTokens = append(editTokens, tokens)
	}
	// What a respelling may not touch: every value the wiring writes and every
	// located value. An edit whose pointer does not read is kept, for the writer
	// to refuse.
	reserved := append([][]string{}, editTokens...)
	for _, c := range located {
		if tokens, err := pointerTokens(c.Pointer); err == nil {
			reserved = append(reserved, tokens)
		}
	}
	var respell respelling
	all := edits
	for _, e := range respelled {
		if tokens, err := pointerTokens(e.Pointer); err == nil && touchesAny(tokens, reserved) {
			respell.left++
			continue
		}
		if respell.written == 0 {
			all = append([]Edit{}, edits...)
		}
		all = append(all, e)
		respell.written++
	}
	type verdict int
	const (
		wired verdict = iota
		referenced
		emptied
		carried
	)
	verdicts := make([]verdict, len(located))
	var toEmpty []Credential
	var emptyAt []int
	for i, c := range located {
		switch {
		case editWrites(editTokens, c.Pointer):
			verdicts[i] = wired
		case references && wholeReference(tree, c):
			verdicts[i] = referenced
		case removal.empties(c.Class):
			verdicts[i] = emptied
			toEmpty = append(toEmpty, c)
			emptyAt = append(emptyAt, i)
		default:
			verdicts[i] = carried
		}
	}
	out, changed, err := writer.write(source, tree, all, toEmpty)
	if err != nil {
		return nil, nil, respelling{}, err
	}
	written := make([]bool, len(located))
	for j, i := range emptyAt {
		written[i] = changed[j]
	}
	var diags []aggrega.Diagnostic
	for i, c := range located {
		ref := refOf(c.Node)
		switch {
		case verdicts[i] == emptied && written[i]:
			diags = append(diags, aggrega.Diagnostic{Severity: "warn", Code: "credential-emptied",
				Message: emptiedMessage(c, ref, where), SourceRef: ref})
		case verdicts[i] == carried && holdsValue(tree, c):
			diags = append(diags, aggrega.Diagnostic{Severity: "warn", Code: "credential-carried",
				Message: carriedMessage(c, ref, where), SourceRef: ref})
		}
	}
	return out, diags, respell, nil
}

// respelling counts the edits of a platform's Respell that carryCredentials
// wrote and those it left out.
type respelling struct{ written, left int }

// editWrites reports whether one of the edits, given by their reference tokens,
// writes the value pointer addresses: an edit at that value, or at a value
// holding it. A pointer that does not read as one is written by none.
func editWrites(edits [][]string, pointer string) bool {
	tokens, err := pointerTokens(pointer)
	if err != nil {
		return false
	}
	for _, e := range edits {
		if covers(e, tokens) {
			return true
		}
	}
	return false
}

// touchesAny reports whether the value tokens address is one of others, holds
// one of them, or lies inside one of them.
func touchesAny(tokens []string, others [][]string) bool {
	for _, o := range others {
		if covers(o, tokens) || covers(tokens, o) {
			return true
		}
	}
	return false
}

// covers reports whether the value the reference tokens outer address is the
// value inner addresses or holds it: outer is the start of inner.
func covers(outer, inner []string) bool {
	return len(outer) <= len(inner) && slices.Equal(outer, inner[:len(outer)])
}

// wholeReference reports whether the value a located credential addresses in
// tree is wholly a variable reference: its pointer reaches a value there, and
// every value it reaches is a JSON string [IsVariableReference] reads as one. A
// member written twice, once as a reference and once with a value, holds that
// value.
func wholeReference(tree json.RawMessage, c Credential) bool {
	spans, err := valueSpans(tree, c.Pointer)
	if err != nil || len(spans) == 0 {
		return false
	}
	for _, s := range spans {
		var text string
		if json.Unmarshal(tree[s.start:s.end], &text) != nil || !IsVariableReference(text) {
			return false
		}
	}
	return true
}

// holdsValue reports whether the value a located credential addresses in tree
// holds something to carry: some value its pointer reaches there is neither "",
// nor null, nor what its Value would write in its place.
func holdsValue(tree json.RawMessage, c Credential) bool {
	spans, err := valueSpans(tree, c.Pointer)
	if err != nil {
		return false
	}
	for _, s := range spans {
		v := tree[s.start:s.end]
		if string(v) == `""` || string(v) == "null" || (c.Value != nil && bytes.Equal(v, c.Value)) {
			continue
		}
		return true
	}
	return false
}

// emptiedMessage reports a credential value written empty, by its node — written
// ref — and its field, never by its value: with the credential's Value set, the
// parts inside the value its Detail names; with only its Detail set, the whole
// value, and why; with neither, a value its format marks as a credential as
// such, and any other the whole value, and why a value of its class can hold
// one. where names where the value is written. It says what is written of that
// one value and nothing of the rest of the document, which can hold a credential
// in a place no format marks.
func emptiedMessage(c Credential, ref, where string) string {
	switch {
	case c.Value != nil:
		return fmt.Sprintf("%s of node %s is written with its %s empty %s", c.Field, ref,
			firstOf(c.Detail, "credential values"), where)
	case c.Detail != "":
		return fmt.Sprintf("the whole value of %s of node %s is written empty %s, since %s", c.Field, ref, where,
			c.Detail)
	case c.Class != CredentialMarked:
		return fmt.Sprintf("the whole value of %s of node %s is written empty %s, since %s", c.Field, ref, where,
			classReason(c.Class))
	default:
		return fmt.Sprintf("the value of %s, which node %s marks as a credential, is written empty %s", c.Field, ref,
			where)
	}
}

// classReason says why a value of class can hold a credential, of the field
// holding it.
func classReason(class CredentialClass) string {
	switch class {
	case CredentialServerMember:
		return "the field belongs to an MCP server's configuration, which can carry a key"
	case CredentialUnmarked:
		return "its format marks the field as no credential, though it can carry one"
	}
	return "its format marks the field as a credential"
}

// carriedMessage reports a credential value carried as written, by its node —
// written ref — and its field, never by its value or any part of it: why a
// value of its class can hold a credential, and which values of
// empty_credentials write it empty. With the credential's Value set, it names
// the parts inside the value its Detail names, the parts that setting writes
// empty.
func carriedMessage(c Credential, ref, where string) string {
	why := classReason(c.Class)
	if c.Value != nil {
		what := firstOf(c.Detail, "credential values")
		return fmt.Sprintf("%s of node %s is carried with its %s as written %s: %s, and setting %s to %s writes its %s "+
			"empty", c.Field, ref, what, where, why, emptyCredentialsSetting, levelEmptying(c.Class), what)
	}
	return fmt.Sprintf("the value of %s of node %s is carried as written %s: %s, and setting %s to %s writes it empty",
		c.Field, ref, where, why, emptyCredentialsSetting, levelEmptying(c.Class))
}
