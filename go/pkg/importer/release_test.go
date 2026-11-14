// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// aggregateFile returns the content of the drafted Aggregate's file out holds,
// empty where it holds none.
func aggregateFile(out aggrega.ImportOutput) string {
	for _, f := range out.Files {
		if f.Path == "aggregate.yaml" {
			return f.Content
		}
	}
	return ""
}

// withHintsOnly returns the settings document given — "" for none — asking for
// the hints-only reuse mode besides what it asks for, so a case reading what a
// drafted part renders as reads the draft it was written against.
func withHintsOnly(settings string) string {
	if settings == "" {
		return hintsOnly
	}
	return strings.Replace(settings, "{", `{"reuse_mode":"hints-only",`, 1)
}

// releaseHead is the head every drafted Release of the test's format opens with,
// named name, its manifest declared empty beneath the comment saying why.
func releaseHead(name string) string {
	return "apiVersion: aggrega.ai/v1beta1\n" +
		"kind: Release\n" +
		"metadata:\n" +
		"  name: " + name + "\n" +
		"  # environment is a drafted default, since no source names the environment this Release deploys to; " +
		"change it to name the one it deploys to.\n" +
		"  environment: development\n" +
		"  version: 0.1.0\n" +
		"  description: Draft Release imported from a source in the testflow format.\n" +
		"spec:\n" +
		"  aggregate: " + name + "\n" +
		"  # manifest: left empty, since the import was given no target; an import given a target drafts the " +
		"Manifest and names it here.\n" +
		"  manifest: \"\"\n"
}

// TestDraftReleaseWritesTheReleaseOfTheDraft pins, byte for byte, the Release
// DraftRelease writes, one row per shape a draft hands it: named as DraftName
// names the Aggregate it deploys, under the target_namespace setting; its
// environment development beneath the comment saying it is a drafted default;
// manifest declared empty beneath the comment saying why, the two lines the host
// replaces with the drafted Manifest's name; and the entries — each with what
// it leaves for the Release above it, under one heading, an item with a path and
// a text written "<path>, <text>" and one with either alone written as that
// alone; then its comments, a comment holding a line feed written as two comment
// lines and an empty one as a bare hash; and its lines beneath its key, an entry
// with no line written as an empty mapping — under components, or,
// where one is a module, under the one boundary the draft names, the instances
// under its components and the modules under its aggregates, each in the order
// given. Every row is written twice, and must write the same bytes both times.
//
// 🔴 PLANTED DEFECTS, each in DraftRelease (release.go):
//
//   - drop the line writing the environment: every row fails;
//   - write every entry under components whatever its Module says: the module
//     rows fail.
func TestDraftReleaseWritesTheReleaseOfTheDraft(t *testing.T) {
	t.Parallel()
	in := aggrega.ImportInput{Format: testFormatName, Name: "support.json"}
	for _, row := range []struct {
		scenario string
		in       aggrega.ImportInput
		titles   []string
		draft    ReleaseDraft
		want     string
	}{
		{"no entry", in, nil, ReleaseDraft{},
			releaseHead("imported/support") + "  components: {}\n"},
		{"entries with comments and lines", in, nil, ReleaseDraft{Entries: []ReleaseEntry{
			{Key: "api", Left: []LeftItem{{Path: "infra.port", Text: "the port it listens on"}, {Path: "infra.region"},
				{Text: "the value of its token"}}, Comments: []string{"The service api."},
				Lines: []string{"infra:", "  image: \"nginx:1\"", "features:", "  token:", "    type: core/secret-value"}},
			{Key: "cache"},
			{Key: "db", Comments: []string{"two\nlines", ""}},
		}},
			releaseHead("imported/support") +
				"  components:\n" +
				"    # Left for the Release, since the source cannot supply it:\n" +
				"    #   - infra.port, the port it listens on\n" +
				"    #   - infra.region\n" +
				"    #   - the value of its token\n" +
				"    # The service api.\n" +
				"    api:\n" +
				"      infra:\n" +
				"        image: \"nginx:1\"\n" +
				"      features:\n" +
				"        token:\n" +
				"          type: core/secret-value\n" +
				"    cache: {}\n" +
				"    # two\n" +
				"    # lines\n" +
				"    #\n" +
				"    db: {}\n"},
		{"a title and a namespace", aggrega.ImportInput{Format: testFormatName, Name: "support.json",
			Settings: json.RawMessage(`{"target_namespace":"team-a"}`)}, []string{"Support Chat"},
			ReleaseDraft{Entries: []ReleaseEntry{{Key: "chat"}}},
			releaseHead("team-a/support-chat") + "  components:\n    chat: {}\n"},
		{"a module alone", in, nil, ReleaseDraft{Boundary: "platform", Entries: []ReleaseEntry{
			{Key: "dify", Module: true, Comments: []string{"The installation."}},
		}},
			releaseHead("imported/support") +
				"  boundaries:\n" +
				"    platform:\n" +
				"      aggregates:\n" +
				"        # The installation.\n" +
				"        dify: {}\n"},
		{"instances and a module, no boundary named", in, nil, ReleaseDraft{Entries: []ReleaseEntry{
			{Key: "web", Left: []LeftItem{{Path: "infra.image", Text: "the image running it"}}},
			{Key: "mod", Module: true, Lines: []string{"components:", "  api: {}"}},
			{Key: "db"},
		}},
			releaseHead("imported/support") +
				"  boundaries:\n" +
				"    default:\n" +
				"      components:\n" +
				"        # Left for the Release, since the source cannot supply it:\n" +
				"        #   - infra.image, the image running it\n" +
				"        web: {}\n" +
				"        db: {}\n" +
				"      aggregates:\n" +
				"        mod:\n" +
				"          components:\n" +
				"            api: {}\n"},
	} {
		first := DraftRelease(row.in, row.draft, row.titles...)
		again := DraftRelease(row.in, row.draft, row.titles...)
		if first.Body != row.want {
			t.Errorf("%s: DraftRelease wrote\n%s\nwant\n%s", row.scenario, first.Body, row.want)
		}
		if again.Body != first.Body {
			t.Errorf("%s: DraftRelease wrote two byte sequences:\n%s\nand\n%s", row.scenario, first.Body, again.Body)
		}
		if name := DraftName(row.in, row.titles...); first.Kind != "Release" || first.Mode != "draft" || first.Name != name {
			t.Errorf("%s: DraftRelease returned the %s %s %q, want the drafted Release %q", row.scenario, first.Mode,
				first.Kind, first.Name, name)
		}
	}
}

// TestConnectionCredentialNamesTheCredentialOnItsConnection pins the item an
// entry leaves for the credential its instance presents on one connection: the
// path the Release attaches it at under that connection, and the sentence naming
// the instance, the target and the socket.
func TestConnectionCredentialNamesTheCredentialOnItsConnection(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		scenario                   string
		key, socket, alias, target string
		want                       LeftItem
	}{
		{"an alias naming its target", "agent", "model", "gpt-4o", "gpt-4o",
			LeftItem{Path: "uses.model.gpt-4o.features.credential",
				Text: "the credential agent presents to gpt-4o on its model connection"}},
		{"an alias apart from its target", "flow", "tools", "docs", "docs-search",
			LeftItem{Path: "uses.tools.docs.features.credential",
				Text: "the credential flow presents to docs-search on its tools connection"}},
	} {
		if got := ConnectionCredential(row.key, row.socket, row.alias, row.target); got != row.want {
			t.Errorf("%s: ConnectionCredential wrote %+v, want %+v", row.scenario, got, row.want)
		}
	}
}

// TestRenderWritesTheDraftedReleaseBesideTheAggregate hands Render an importer's
// own entities — a drafted Component, a drafted Aggregate and the Release
// DraftRelease wrote — and requires the Release written as release.yaml, after
// the Aggregate, as the entities give them; under render_mode llm no draft file
// is written, the Release's included.
func TestRenderWritesTheDraftedReleaseBesideTheAggregate(t *testing.T) {
	t.Parallel()
	in := aggrega.ImportInput{Format: testFormatName}
	release := DraftRelease(in, ReleaseDraft{Entries: []ReleaseEntry{{Key: "a"}}})
	entities := []aggrega.EmittedEntity{
		{Kind: "Component", Name: "imported/component/a", Mode: "draft", Body: "kind: Component\n"},
		{Kind: "Aggregate", Name: "imported/flow", Mode: "draft", Body: "kind: Aggregate\n"},
		release,
	}
	var paths []string
	for _, f := range Render(nil, entities, nil) {
		paths = append(paths, f.Path)
		if f.Path == ReleaseFileName && f.Content != release.Body {
			t.Errorf("%s reads\n%s\nwant the Release's body\n%s", f.Path, f.Content, release.Body)
		}
	}
	if want := []string{"components/imported-component-a.yaml", "aggregate.yaml", ReleaseFileName}; !slices.Equal(paths,
		want) {
		t.Errorf("Render wrote %v, want %v", paths, want)
	}
	for _, f := range Render(json.RawMessage(`{"render_mode":"llm"}`), entities, nil) {
		if f.Path == ReleaseFileName {
			t.Errorf("render_mode llm wrote %s", f.Path)
		}
	}
}

// releaseWriter is the file holding the package's one Release writer.
const releaseWriter = "release.go"

// releaseComposedApart is one place a source composes a Release other than
// through the package's writer: where, and how.
type releaseComposedApart struct {
	at, how string
}

// releasesComposedApart parses one Go source and returns every place it composes
// a Release itself: a string literal holding a Release's kind line, or an
// EmittedEntity literal — written with its package, bare, or elided inside a
// composite of them — whose Kind is Release and whose Mode is not reference.
func releasesComposedApart(t *testing.T, at string, src []byte) []releaseComposedApart {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, at, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", at, err)
	}
	var out []releaseComposedApart
	ast.Inspect(file, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.BasicLit:
			if e.Kind == token.STRING && strings.Contains(literalText(e), "kind: Release") {
				out = append(out, releaseComposedApart{at: at + ":" + strconv.Itoa(fset.Position(e.Pos()).Line),
					how: "a string literal writing a Release's kind"})
			}
		case *ast.CompositeLit:
			if !emittedEntityType(e.Type) {
				return true
			}
			fields := map[string]ast.Expr{}
			for _, elt := range e.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if key, ok := kv.Key.(*ast.Ident); ok {
						fields[key.Name] = kv.Value
					}
				}
			}
			if literalText(fields["Kind"]) == "Release" && literalText(fields["Mode"]) != "reference" {
				out = append(out, releaseComposedApart{at: at + ":" + strconv.Itoa(fset.Position(e.Pos()).Line),
					how: "an EmittedEntity of kind Release"})
			}
		}
		return true
	})
	return out
}

// TestEveryReleaseOfThePackageIsWrittenByDraftRelease parses the package's own
// source and holds every Release it drafts to its one writer: outside release.go
// no string literal writes a Release's kind line and no EmittedEntity literal is
// a drafted Release, and inside it — where both are the writer's own — exactly
// one EmittedEntity literal is, so a scan that read nothing fails. The check is first run over a planted source
// holding each form, each required to be found, and a call of DraftRelease,
// required to pass.
//
// The compiler's importers are held to the same rule by a twin check,
// TestEveryImporterDraftsItsReleaseThroughTheKit in the compiler's
// plugins/internal/guestwire: this package is a module of its own, published
// apart, and imports nothing from the compiler's module, so the two checks are
// written once on each side.
//
// 🔴 PLANTED DEFECT: in (*platformDraft).release (platform.go), return
// aggrega.EmittedEntity{Kind: "Release", Name: draftName(d.settings, in, d.titles),
// Mode: "draft", Body: "kind: Release\n"}: this test reddens naming platform.go
// twice, for the literal and for the kind line.
func TestEveryReleaseOfThePackageIsWrittenByDraftRelease(t *testing.T) {
	t.Parallel()
	const planted = `package p

func (a *b) text() string {
	return "apiVersion: v\nkind: Release\n"
}

func (a *b) selected(in aggrega.ImportInput) aggrega.EmittedEntity {
	return aggrega.EmittedEntity{Kind: "Release", Name: draftName(a.settings, in, nil), Mode: "draft"}
}

func (a *b) elided(in aggrega.ImportInput) []EmittedEntity {
	return []EmittedEntity{{Kind: "Release", Name: draftName(a.settings, in, nil), Mode: "draft"}}
}

func (a *b) written(in aggrega.ImportInput) aggrega.EmittedEntity {
	return DraftRelease(in, ReleaseDraft{})
}
`
	var hows []string
	for _, c := range releasesComposedApart(t, "planted.go", []byte(planted)) {
		hows = append(hows, c.how)
	}
	if want := []string{"a string literal writing a Release's kind", "an EmittedEntity of kind Release",
		"an EmittedEntity of kind Release"}; !slices.Equal(hows, want) {
		t.Fatalf("the check found %v in the planted source; want the kind line and both literals, and nothing for "+
			"the call of DraftRelease", hows)
	}

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob the package's source: %v", err)
	}
	sort.Strings(paths)
	written := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, c := range releasesComposedApart(t, path, src) {
			if path == releaseWriter {
				if c.how == "an EmittedEntity of kind Release" {
					written++
				}
				continue
			}
			t.Errorf("%s composes a Release with %s; every Release the package drafts is written by DraftRelease, in "+
				"%s", c.at, c.how, releaseWriter)
		}
	}
	if written != 1 {
		t.Errorf("found %d drafted Release literal(s) in %s, want DraftRelease's one; the scan is not reading what it "+
			"checks", written, releaseWriter)
	}
}
