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
	// Left are the values the entry leaves for the Release to supply, since no
	// source holds them, in the order given: written first above the entry, under
	// one heading, one comment line each, as [LeftItem] describes.
	Left []LeftItem
	// Comments are written above the entry, after what it leaves for the Release,
	// one comment line each, at the entry's
	// own indentation: each is the comment's text, without the hash, and a text
	// holding a line feed is written as one comment line per line it holds. A
	// Release is not evaluated as a template, so each is written as it is.
	Comments []string
	// Lines are the entry's body, one YAML line each, indented as the entry's
	// direct members are and written beneath its key: infra:, features: and the
	// lines nested under them, each written with the indentation it takes relative
	// to the entry. An entry holding no line is written as an empty mapping.
	Lines []string
}

// LeftItem is one value an entry of a drafted Release leaves for the Release to
// supply, as [ReleaseEntry.Left] lists them.
type LeftItem struct {
	// Path is the key path, relative to the entry, the Release writes the value
	// at — infra.image, features.admin_password, a connection's
	// uses.<socket>.<alias>.features.credential — empty where the item names no
	// one key.
	Path string
	// Text says what the value is, completing "<Path>, " where Path is set and
	// standing alone where it is not; empty writes the path alone. It is written
	// into a comment as it is, so it carries nothing read from a source a comment
	// cannot carry.
	Text string
}

// ConnectionCredential is the value an entry keyed key leaves for the Release
// where the instance presents a credential on the connection its socket draws
// under alias, reaching the instance keyed target: the credential the Release
// attaches on that connection. Every importer leaving such a credential leaves it
// through this, so it is named one way.
func ConnectionCredential(key, socket, alias, target string) LeftItem {
	return LeftItem{Path: "uses." + socket + "." + alias + ".features.credential",
		Text: "the credential " + key + " presents to " + target + " on its " + socket + " connection"}
}

// leftHeading opens the comment listing what an entry leaves for the Release.
const leftHeading = "Left for the Release, since the source cannot supply it:"

// leftLine is the comment line naming one item an entry leaves for the Release.
func leftLine(item LeftItem) string {
	switch {
	case item.Path == "":
		return "  - " + item.Text
	case item.Text == "":
		return "  - " + item.Path
	}
	return "  - " + item.Path + ", " + item.Text
}

// DraftRelease writes the Release an import drafts beside its Aggregate, named
// as [DraftName] names the drafted Aggregate for the request in and the titles
// its sources give themselves, and deploying that Aggregate. It is the one
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
// drafts the Manifest and writes its name in place of both lines — then holds the
// entries: under components
// where none is a module, and otherwise under the one boundary
// [ReleaseDraft.Boundary] names, the instances under its components and the
// modules under its aggregates, each in the order given. Every entry is written
// with what it leaves for the Release above it — "Left for the Release, since the
// source cannot supply it:", then one "  - <path>, <text>" line per item — then
// its comments, then its key and its lines beneath it, an entry with no line as
// an empty mapping. The text is written by hand, so the key order is
// fixed and one draft writes one byte sequence.
//
// It returns the Release as a drafted entity, which the package's renderer writes
// as [ReleaseFileName].
func DraftRelease(in aggrega.ImportInput, draft ReleaseDraft, titles ...string) aggrega.EmittedEntity {
	name := draftName(parseSettings(in.Settings), in, titles)
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

	var components, modules []ReleaseEntry
	for _, e := range draft.Entries {
		if e.Module {
			modules = append(modules, e)
		} else {
			components = append(components, e)
		}
	}
	if len(modules) == 0 {
		writeReleaseEntries(&b, "  ", "components", components)
	} else {
		boundary := draft.Boundary
		if boundary == "" {
			boundary = "default"
		}
		b.WriteString("  boundaries:\n")
		b.WriteString("    " + boundary + ":\n")
		if len(components) > 0 {
			writeReleaseEntries(&b, "      ", "components", components)
		}
		writeReleaseEntries(&b, "      ", "aggregates", modules)
	}
	return aggrega.EmittedEntity{Kind: "Release", Name: name, Mode: "draft", Body: b.String()}
}

// leftLines is the comment line of each item, in order.
func leftLines(items []LeftItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, leftLine(item))
	}
	return out
}

// writeReleaseEntries writes the mapping key, at indent, holding entries: each
// entry's comments and key two spaces further in, and its lines two spaces
// further still; key: {} where there is no entry.
func writeReleaseEntries(b *strings.Builder, indent, key string, entries []ReleaseEntry) {
	if len(entries) == 0 {
		b.WriteString(indent + key + ": {}\n")
		return
	}
	b.WriteString(indent + key + ":\n")
	at := indent + "  "
	for _, e := range entries {
		comments := e.Comments
		if len(e.Left) > 0 {
			comments = append([]string{leftHeading}, leftLines(e.Left)...)
			comments = append(comments, e.Comments...)
		}
		for _, comment := range comments {
			for _, line := range strings.Split(comment, "\n") {
				if line == "" {
					b.WriteString(at + "#\n")
					continue
				}
				b.WriteString(at + "# " + line + "\n")
			}
		}
		if len(e.Lines) == 0 {
			b.WriteString(at + e.Key + ": {}\n")
			continue
		}
		b.WriteString(at + e.Key + ":\n")
		for _, line := range e.Lines {
			if line == "" {
				b.WriteString("\n")
				continue
			}
			b.WriteString(at + "  " + line + "\n")
		}
	}
}
