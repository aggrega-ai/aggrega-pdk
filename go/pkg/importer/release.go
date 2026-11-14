// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"strings"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// ReleaseFileName is the path the drafted Release is written under, beside the
// drafted Aggregate's aggregate.yaml.
const ReleaseFileName = "release.yaml"

// ManifestFileName is the path the Manifest a draft writes, where it writes one
// ([DraftRelease]), is written under, beside the drafted Release.
const ManifestFileName = "manifest.yaml"

// draftedEnvironment is the environment every drafted Release names: a default
// the draft states, since no source names the environment a Release deploys to.
const draftedEnvironment = "development"

// ReleaseDraft is the Release an import drafts beside its Aggregate, as
// [DraftRelease] writes it: one entry per instance of the drafted Aggregate, in
// the order the Aggregate declares them, each carrying what the import knows of
// what the Release supplies for that instance.
type ReleaseDraft struct {
	// Boundary is the name of the boundary a Release holding a module entry
	// places every entry in: a Release places a module only under a boundary, so
	// such a Release is written with that one boundary rather than with
	// components alone. Empty names it default. A Release holding no module entry
	// declares no boundary, and this is not read.
	Boundary string
	// Parameters are the parameters of the drafted Aggregate the Release fulfills
	// that no source supplies, each written under the Release's spec.parameters
	// as its Path holding an empty string, with its Text in a comment directly
	// above it, in the order given. Path is the parameter's name.
	Parameters []LeftItem
	// SecretParameters are the parameters of the drafted Aggregate the Release
	// fulfills that hold a secret no source supplies, each written under the
	// Release's spec.parameters, after Parameters, as its Path, in the order given,
	// for the target in the form [Secret.Form] gives it: where the form names the
	// secret by its resource, holding an empty string beneath the comment a left
	// parameter is written with; otherwise holding the variable reference
	// ${Variable}, beneath a comment saying it is passed by reference and where it
	// is read from. Path is the parameter's name, and Text completes "<Path>, "
	// in either comment.
	SecretParameters []Secret
	// Entries are the Release's entries, one per instance of the drafted
	// Aggregate, in its order.
	Entries []ReleaseEntry
}

// ReleaseEntry is one entry of a drafted Release, as [ReleaseDraft] lists them.
type ReleaseEntry struct {
	// Key is the key the instance is written under in the drafted Aggregate.
	Key string
	// Module says the instance is an entry of the drafted Aggregate's
	// aggregates — a module the Aggregate composes — which the Release routes
	// under its boundary's aggregates rather than naming under components.
	Module bool
	// Realization, where set, is the name of the Recipe that realizes this
	// instance on the import's target, where the target offers several for its
	// type and the importer knows which one the instance needs, such as the
	// realization running a server from its command line rather than from an
	// image. A draft given a target maps the instance to <target>/<Realization>
	// by its key in the Manifest it writes; one given none maps nothing.
	Realization string
	// Values are the values the draft sets in the entry, each written at its
	// path, in the order given, after what Lines write.
	Values []ReleaseValue
	// Left are the values the entry leaves for the Release to supply, since no
	// source holds them, in the order given, as [LeftItem] describes: each with a
	// path holding a string is written at it holding an empty string, after the
	// values, with a comment naming it directly above it; each other — one naming
	// no key, or one whose value is no string — is written into one comment above
	// the entry, under one heading, one line each.
	Left []LeftItem
	// Secrets are the secrets the entry attaches, each written at its path as
	// [Secret] describes, after what it leaves for the Release.
	Secrets []Secret
	// Comments are written above the entry, after what it leaves for the Release,
	// one comment line each, at the entry's
	// own indentation: each is the comment's text, without the hash, and a text
	// holding a line feed is written as one comment line per line it holds. A
	// Release is not evaluated as a template, so each is written as it is.
	Comments []string
	// Lines are the entry's body, one YAML line each, indented as the entry's
	// direct members are and written beneath its key: infra:, features: and the
	// lines nested under them, each written with the indentation it takes relative
	// to the entry. The values, the left items and the secrets are written into
	// the mappings they name, a mapping Lines write included, so a key is written
	// once. An entry holding nothing to write is written as an empty mapping.
	//
	// The two inputs are deliberate, not a second way of writing one thing: Lines
	// carry what an importer writes in a shape of its own — a list, a value
	// across several lines — which no key path addresses, and the keyed fields
	// carry what the package itself decides how to write — a value it sets, one
	// it leaves empty, a secret's form for the target. A keyed field whose key
	// Lines already write is not written again: a left item so passed over is
	// named in the comment above the entry, and a value or a secret so passed over
	// is the one Lines write.
	Lines []string
}

// ReleaseValue is a value the draft sets in an entry of a drafted Release, as
// [ReleaseEntry.Values] lists them.
type ReleaseValue struct {
	// Path is the key path, relative to the entry, the value is written at, such
	// as infra.image.
	Path string
	// Value is the value, written as it is as a YAML scalar, so the caller writes
	// it as one: a string read from a source through a writer quoting it.
	Value string
	// Comment, where set, is written directly above the value's key, one comment
	// line per line it holds.
	Comment string
}

// LeftItem is one value an entry of a drafted Release leaves for the Release to
// supply, as [ReleaseEntry.Left] lists them.
type LeftItem struct {
	// Path is the key path, relative to the entry, the Release writes the value
	// at — infra.image, infra.owner_email — empty where the item names no one key.
	// The value at a path is a scalar: a secret the Release attaches is a
	// [Secret], never a left item.
	Path string
	// Text says what the value is, completing "<Path>, " where Path is set and
	// standing alone where it is not; empty writes the path alone. It is written
	// into a comment as it is, so it carries nothing read from a source a comment
	// cannot carry.
	Text string
	// NonString says the value at Path is not a string — a boolean, a list — so
	// an empty string written there would be neither a value still owed nor a
	// valid one. An entry's left item so marked is never written into its body: it
	// is named, its path and its text, in the comment above the entry, as an item
	// naming no key is, and Text says what the Release writes there. It is not
	// read for a parameter, which [ReleaseDraft.Parameters] writes as a string.
	NonString bool
}

// LeftImage is the value an entry leaves for the Release where no source names
// the image its instance runs: infra.image, with text saying what the image is
// or why no source names it. Every importer leaving the image leaves it through
// this, so it is named one way.
func LeftImage(text string) LeftItem {
	return LeftItem{Path: "infra.image", Text: text}
}

// leftHeading opens the comment listing what an entry leaves for the Release,
// and the comment above each value it leaves written empty.
const leftHeading = "Left for the Release, since the source cannot supply it:"

// leftLine is the text naming one item an entry leaves for the Release.
func leftLine(item LeftItem) string {
	switch {
	case item.Path == "":
		return item.Text
	case item.Text == "":
		return item.Path
	}
	return item.Path + ", " + item.Text
}

// DraftRelease writes the Release an import drafts beside its Aggregate,
// aggregate, the drafted Aggregate itself, whose name is the one DraftName gave
// it: the Release takes that name and deploys it. It is the one
// writer of a drafted Release: every importer built on the package drafts its
// Release through it, the package's own drafts included, so one source's Release
// is written one way whichever importer drafts it.
//
// The Release declares, in order: its name; its environment, development, with
// the comment above it saying it is a drafted default the author may change; its
// version, 0.1.0; and a description naming the request's format. Its spec names
// the drafted Aggregate, then declares manifest empty, beneath a comment line
// beginning "# manifest:" saying why — a Release's spec requires the key, and an
// empty one is a value still owed: where the import is given a target, the host
// drafts the Manifest and writes its name in place of both lines — then the
// parameters the draft leaves for the Release, each empty, and those holding a
// secret, each as [ReleaseDraft.SecretParameters] says, then the entries:
// under components where none is a module, and otherwise under the one boundary
// [ReleaseDraft.Boundary] names, the instances under its components and the
// modules under its aggregates, each in the order given. Every entry is written
// with what it leaves for the Release that names no key above it — "Left for the
// Release, since the source cannot supply it:", then one "  - <path>, <text>"
// line per item — then its comments, then its key and its body beneath it: its
// lines, its values, each left item naming a key that holds a string as that key
// holding an empty string beneath a comment beginning with the same heading — a
// left item whose value is no string is named in the comment above the entry
// instead, so a typed key is never drafted as an empty string — and its secrets, each
// written into the mapping its path names; an entry with nothing to write as an
// empty mapping. An item whose key the body already writes, or whose path runs
// through a value or a list the body writes, is not written into the body; a left
// item so passed over is named in the comment above the entry instead. The text
// is written by hand, so the key order is fixed and one draft writes one byte
// sequence.
//
// Each secret, an entry's or a parameter's, is written for the request's target in
// the form [Secret.Form] gives it. Where a form names a realization — a secret named by reference on
// helm, whose realization reading the name as a Secret already in the namespace
// is one of several the catalog offers — or, the import naming a target, an
// entry names its [ReleaseEntry.Realization], the draft also writes a Manifest,
// named and versioned as the Release, mapping each such entry's instance by its
// key to <target>/<realization>, in the order of the entries, and each such
// secret type to its realization, in the order the secrets first name it, the
// parameters' first; the
// host completes the rest of the Manifest. It returns the Release, then that
// Manifest where there is one, as drafted entities, which the package's renderer
// writes as [ReleaseFileName] and [ManifestFileName].
func DraftRelease(in aggrega.ImportInput, aggregate aggrega.EmittedEntity, draft ReleaseDraft) []aggrega.EmittedEntity {
	name := aggregate.Name
	var b strings.Builder
	b.WriteString("apiVersion: " + apiVersion + "\n")
	b.WriteString("kind: Release\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: " + name + "\n")
	b.WriteString("  # environment is a drafted default, since no source names the environment this Release deploys " +
		"to; change it to name the one it deploys to.\n")
	b.WriteString("  environment: " + draftedEnvironment + "\n")
	b.WriteString("  version: 0.1.0\n")
	b.WriteString("  description: Draft Release imported from a source in the " + in.Format + " format.\n")
	b.WriteString("spec:\n")
	b.WriteString("  aggregate: " + name + "\n")
	b.WriteString("  # manifest: left empty, since the import was given no target; an import given a target drafts the " +
		"Manifest and names it here.\n")
	b.WriteString("  manifest: \"\"\n")
	if len(draft.Parameters) > 0 || len(draft.SecretParameters) > 0 {
		b.WriteString("  parameters:\n")
		for _, param := range draft.Parameters {
			for _, n := range leftParameterNodes(param) {
				n.write(&b, "    ", "    ")
			}
		}
		for _, s := range draft.SecretParameters {
			for _, n := range secretParameterNodes(s, in.Target) {
				n.write(&b, "    ", "    ")
			}
		}
	}

	var components, modules []ReleaseEntry
	var realized, named []string
	realization := map[string]string{}
	realize := func(s Secret) {
		if form := s.Form(in.Target); form.Realization != "" && realization[form.Type] == "" {
			realized = append(realized, form.Type)
			realization[form.Type] = form.Realization
		}
	}
	for _, s := range draft.SecretParameters {
		realize(s)
	}
	for _, e := range draft.Entries {
		if e.Module {
			modules = append(modules, e)
		} else {
			components = append(components, e)
		}
		if e.Realization != "" && in.Target != "" {
			named = append(named, e.Key+": "+in.Target+"/"+e.Realization)
		}
		for _, s := range e.Secrets {
			realize(s)
		}
	}
	if len(modules) == 0 {
		writeReleaseEntries(&b, "  ", "components", components, in.Target)
	} else {
		boundary := draft.Boundary
		if boundary == "" {
			boundary = "default"
		}
		b.WriteString("  boundaries:\n")
		b.WriteString("    " + boundary + ":\n")
		if len(components) > 0 {
			writeReleaseEntries(&b, "      ", "components", components, in.Target)
		}
		writeReleaseEntries(&b, "      ", "aggregates", modules, in.Target)
	}
	out := []aggrega.EmittedEntity{{Kind: "Release", Name: aggregate.Name, Mode: "draft", Body: b.String()}}
	if len(realized) > 0 || len(named) > 0 {
		out = append(out, aggrega.EmittedEntity{Kind: "Manifest", Name: name, Mode: "draft",
			Body: realizedManifest(name, named, realized, realization)})
	}
	return out
}

// leftParameterNodes are the lines a drafted Release declares param with under
// its spec.parameters, a parameter left for the Release to fulfill: the
// parameter, keyed by its Path, holding an empty string, beneath a comment
// naming it under the heading every left value is written beneath.
func leftParameterNodes(param LeftItem) []*bodyNode {
	return append(commentNodes(leftHeading+" "+leftLine(param)), &bodyNode{text: param.Path + ": \"\""})
}

// realizedManifest is the Manifest a draft writes where it knows a realization
// only the import can choose, named name: each line of named, an instance's key
// and the Recipe realizing it, under the components' name mappings; and each
// type in types mapped to its realization, under the features' type mappings —
// the mappings only the import chooses, which the host completes with every
// other mapping the drafts need.
func realizedManifest(name string, named, types []string, realization map[string]string) string {
	var b strings.Builder
	b.WriteString("apiVersion: " + apiVersion + "\n")
	b.WriteString("kind: Manifest\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: " + name + "\n")
	b.WriteString("  version: 0.1.0\n")
	b.WriteString("spec:\n")
	b.WriteString("  mappings:\n")
	if len(named) > 0 {
		b.WriteString("    components:\n")
		b.WriteString("      name:\n")
		for _, line := range named {
			b.WriteString("        " + line + "\n")
		}
	}
	if len(types) > 0 {
		b.WriteString("    features:\n")
		b.WriteString("      type:\n")
		for _, typ := range types {
			b.WriteString("        " + typ + ": " + realization[typ] + "\n")
		}
	}
	return b.String()
}

// writeReleaseEntries writes the mapping key, at indent, holding entries: each
// entry's comments and key two spaces further in, and its body two spaces
// further still, its secrets written for target; key: {} where there is no
// entry.
func writeReleaseEntries(b *strings.Builder, indent, key string, entries []ReleaseEntry, target string) {
	if len(entries) == 0 {
		b.WriteString(indent + key + ": {}\n")
		return
	}
	b.WriteString(indent + key + ":\n")
	at := indent + "  "
	for _, e := range entries {
		body, passed := entryBody(e, target)
		comments := e.Comments
		if len(passed) > 0 {
			comments = []string{leftHeading}
			for _, item := range passed {
				comments = append(comments, "  - "+leftLine(item))
			}
			comments = append(comments, e.Comments...)
		}
		for _, comment := range comments {
			for _, n := range commentNodes(comment) {
				n.write(b, at, at)
			}
		}
		if len(body) == 0 {
			b.WriteString(at + e.Key + ": {}\n")
			continue
		}
		b.WriteString(at + e.Key + ":\n")
		for _, n := range body {
			n.write(b, at+"  ", at+"  ")
		}
	}
}

// entryBody is the body of e, as [DraftRelease] writes it for target, and the
// left items it does not write into the body, in their order: those naming no
// key, those whose value is no string, and those whose key the body already
// writes.
func entryBody(e ReleaseEntry, target string) ([]*bodyNode, []LeftItem) {
	body := parseBody(e.Lines)
	for _, v := range e.Values {
		var comments []*bodyNode
		if v.Comment != "" {
			comments = commentNodes(v.Comment)
		}
		place(&body, v.Path, comments, &bodyNode{text: leaf(v.Path) + ": " + v.Value})
	}
	var passed []LeftItem
	for _, item := range e.Left {
		if item.Path == "" || item.NonString {
			passed = append(passed, item)
			continue
		}
		node := &bodyNode{text: leaf(item.Path) + ": \"\""}
		if !place(&body, item.Path, commentNodes(leftHeading+" "+leftLine(item)), node) {
			passed = append(passed, item)
		}
	}
	for _, s := range e.Secrets {
		place(&body, s.Path, nil, secretNode(s, target))
	}
	return body, passed
}

// leaf is the last segment of a key path.
func leaf(path string) string {
	return path[strings.LastIndex(path, ".")+1:]
}

// bodyNode is one line of an entry's body and the lines nested beneath it: a
// key, a key and its value, a list item, a comment, or a line of a value written
// across several lines.
type bodyNode struct {
	// text is the line, without its indentation.
	text string
	// indent, where set, is the line's indentation relative to the entry's body,
	// as Lines wrote it; a line the draft adds is written two spaces in from the
	// line it is nested under.
	indent   *int
	children []*bodyNode
}

// write writes n, and the lines nested beneath it: a line of Lines at base
// followed by the indentation Lines gave it, and a line the draft adds at pad,
// two spaces in from the line it is nested under.
func (n *bodyNode) write(b *strings.Builder, base, pad string) {
	if n.indent != nil {
		pad = base + strings.Repeat(" ", *n.indent)
	}
	if n.text == "" {
		b.WriteString("\n")
	} else {
		b.WriteString(pad + n.text + "\n")
	}
	for _, c := range n.children {
		c.write(b, base, pad+"  ")
	}
}

// key returns the key a line writes, and whether it writes one and whether that
// key holds a value written on the line.
func (n *bodyNode) key() (key string, valued, ok bool) {
	t := n.text
	if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "-") {
		return "", false, false
	}
	if strings.HasSuffix(t, ":") {
		return strings.TrimSuffix(t, ":"), false, true
	}
	if i := strings.Index(t, ": "); i > 0 {
		return t[:i], true, true
	}
	return "", false, false
}

// listItem reports whether a line is an item of a list.
func (n *bodyNode) listItem() bool {
	return strings.HasPrefix(n.text, "-")
}

// parseBody reads lines, an entry's body as [ReleaseEntry.Lines] writes it, as
// lines and the lines nested beneath each, by indentation; a blank line is
// nested with the line before it.
func parseBody(lines []string) []*bodyNode {
	type open struct {
		indent int
		node   *bodyNode
	}
	var roots []*bodyNode
	var stack []open
	for _, line := range lines {
		text := strings.TrimLeft(line, " ")
		indent := len(line) - len(text)
		if text == "" && len(stack) > 0 {
			indent = stack[len(stack)-1].indent
		}
		n := &bodyNode{text: text, indent: &indent}
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, n)
		} else {
			parent := stack[len(stack)-1].node
			parent.children = append(parent.children, n)
		}
		stack = append(stack, open{indent: indent, node: n})
	}
	return roots
}

// place writes node, beneath the comments above it, at path in the body nodes
// hold: into the mapping each segment but the last names, adding each the body
// does not write. It writes nothing, and reports false, where the body already
// writes the last segment's key, or where a segment names a key holding a value
// written on its line, or a mapping holding a list.
func place(nodes *[]*bodyNode, path string, comments []*bodyNode, node *bodyNode) bool {
	segments := strings.Split(path, ".")
	level := nodes
	for i, segment := range segments {
		var found *bodyNode
		for _, n := range *level {
			if n.listItem() {
				return false
			}
			if key, valued, ok := n.key(); ok && key == segment {
				if valued || i == len(segments)-1 {
					return false
				}
				found = n
			}
		}
		if found == nil {
			for _, rest := range segments[i : len(segments)-1] {
				parent := &bodyNode{text: rest + ":"}
				*level = append(*level, parent)
				level = &parent.children
			}
			*level = append(append(*level, comments...), node)
			return true
		}
		level = &found.children
	}
	return false
}

// commentNodes are the comment lines text is written as, one per line it holds,
// an empty line written as a bare hash: the one rule every comment line of a
// drafted Release is written by. A Release is not evaluated as a template, so
// each line is written as it is.
func commentNodes(text string) []*bodyNode {
	var out []*bodyNode
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			out = append(out, &bodyNode{text: "#"})
			continue
		}
		out = append(out, &bodyNode{text: "# " + line})
	}
	return out
}
