// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// This file holds the one decision an import makes over the credential values a
// format locates: which of them the empty_credentials setting writes empty, and
// which are carried as written, each named in a warning that never shows the
// value.

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
// doc being written.
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
	return carryCredentials(removal, doc, doc, nil, located, where, nil)
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
// as; each other value of a class removal empties written empty, located in the
// source's bytes; and each other value carried as written, judged off tree,
// never located in the bytes. refOf names a node in a report, nil naming it as
// located. It returns the written bytes and the diagnostics, as
// [CarryCredentials] describes them. A value to write empty that the bytes do not
// hold is refused with a credentialNotLocated error; an edit the bytes do not
// hold, or bytes that are not one JSON document, with another error.
func carryCredentials(removal credentialRemoval, source []byte, tree json.RawMessage, edits []Edit,
	located []Credential, where string, refOf func(string) string) ([]byte, []aggrega.Diagnostic, error) {
	if refOf == nil {
		refOf = func(node string) string { return node }
	}
	var editTokens [][]string
	for _, e := range edits {
		tokens, err := pointerTokens(e.Pointer)
		if err != nil {
			return nil, nil, fmt.Errorf("the value at %s: %w", e.Pointer, err)
		}
		editTokens = append(editTokens, tokens)
	}
	type verdict int
	const (
		wired verdict = iota
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
		case removal.empties(c.Class):
			verdicts[i] = emptied
			toEmpty = append(toEmpty, c)
			emptyAt = append(emptyAt, i)
		default:
			verdicts[i] = carried
		}
	}
	out, changed, err := rewriteSourceAt(source, edits, toEmpty)
	if err != nil {
		return nil, nil, err
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
	return out, diags, nil
}

// editWrites reports whether one of the edits, given by their reference tokens,
// writes the value pointer addresses: an edit at that value, or at a value
// holding it. A pointer that does not read as one is written by none.
func editWrites(edits [][]string, pointer string) bool {
	tokens, err := pointerTokens(pointer)
	if err != nil {
		return false
	}
	for _, e := range edits {
		if len(e) > len(tokens) {
			continue
		}
		covers := true
		for i := range e {
			if e[i] != tokens[i] {
				covers = false
				break
			}
		}
		if covers {
			return true
		}
	}
	return false
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
