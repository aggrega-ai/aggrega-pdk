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

// TestDraftNameNamesTheDraftAfterTheSource holds DraftName to its order — the
// titles the sources give themselves where they agree, then the name of the
// directory the import was handed, whole, or the stem of the file it was handed,
// then flow — and to the namespace the target_namespace setting names, one row
// per case. A directory's request carries named sources and no primary bytes; a
// file's carries its bytes, with or without named sources beside them. Every row
// is asked twice, and must answer the same both times.
func TestDraftNameNamesTheDraftAfterTheSource(t *testing.T) {
	t.Parallel()
	files := []aggrega.ImportSource{{Name: "compose.yaml", Bytes: []byte("services: {}\n")}}
	for _, row := range []struct {
		scenario string
		in       aggrega.ImportInput
		titles   []string
		want     string
	}{
		{"one title", aggrega.ImportInput{Name: "export.json"}, []string{"Support Chat"}, "imported/support-chat"},
		{"titles agreeing once reduced", aggrega.ImportInput{}, []string{"Support Chat", "support_chat"},
			"imported/support-chat"},
		{"titles disagreeing fall to the file", aggrega.ImportInput{Name: "both.yaml"}, []string{"one", "two"},
			"imported/both"},
		{"a source naming nothing beside one naming itself falls to the file", aggrega.ImportInput{Name: "mixed.json"},
			[]string{"named", ""}, "imported/mixed"},
		{"no title, the file's stem", aggrega.ImportInput{Name: "My Flow.v2.json"}, nil, "imported/my-flow-v2"},
		{"a title reducing to nothing falls to the file", aggrega.ImportInput{Name: "compose.yaml"}, []string{"--- !!"},
			"imported/compose"},
		{"a file's stem reducing to nothing falls to flow", aggrega.ImportInput{Name: ".env"}, nil, "imported/flow"},
		{"a title reducing to nothing and no file, flow", aggrega.ImportInput{}, []string{"!!!"}, "imported/flow"},
		{"no name and no title, flow", aggrega.ImportInput{}, nil, "imported/flow"},
		{"no title, the directory's name", aggrega.ImportInput{Name: "shop", Sources: files}, nil, "imported/shop"},
		{"a directory's name keeps what reads as an extension", aggrega.ImportInput{Name: "my.project", Sources: files},
			nil, "imported/my-project"},
		{"a title names a directory's draft", aggrega.ImportInput{Name: "shop", Sources: files}, []string{"Notes"},
			"imported/notes"},
		{"titles disagreeing fall to the directory", aggrega.ImportInput{Name: "shop", Sources: files},
			[]string{"one", "two"}, "imported/shop"},
		{"a directory's name reducing to nothing falls to flow", aggrega.ImportInput{Name: "__", Sources: files}, nil,
			"imported/flow"},
		{"a file handed beside named sources, the file's stem",
			aggrega.ImportInput{Name: "compose.yaml", Bytes: []byte("services: {}\n"), Sources: files}, nil,
			"imported/compose"},
		{"target_namespace sets the first segment",
			aggrega.ImportInput{Settings: json.RawMessage(`{"target_namespace":"team-a"}`)}, []string{"Support Chat"},
			"team-a/support-chat"},
		{"an empty target_namespace is the default",
			aggrega.ImportInput{Settings: json.RawMessage(`{"target_namespace":""}`)}, nil, "imported/flow"},
		{"a settings document that is not JSON is the default",
			aggrega.ImportInput{Name: "a.json", Settings: json.RawMessage(`not json`)}, nil, "imported/a"},
	} {
		first := DraftName(row.in, row.titles...)
		again := DraftName(row.in, row.titles...)
		if first != row.want {
			t.Errorf("%s: DraftName = %q, want %q", row.scenario, first, row.want)
		}
		if again != first {
			t.Errorf("%s: DraftName answered %q, then %q", row.scenario, first, again)
		}
	}
}

// titled returns served with its Title reading the title given, whatever the
// tree.
func titled(served Format, title string) Format {
	served.Title = func(json.RawMessage) string { return title }
	return served
}

// TestEveryDraftOfTheKitIsNamedAfterTheSource imports through each of the
// package's three drafts — node by node, the platform carrying the flow, and the
// module form — and reads the drafted Aggregate's name off its entity and off
// the metadata.name its file writes: the format's title where it reads one, the
// stem of the file or the whole name of the directory where it reads none, flow
// where its title reduces to nothing and nothing is named, and the namespace the
// target_namespace setting names in front.
//
// 🔴 PLANTED DEFECT: in platformDraft.entity (platform.go), write the name as
// d.settings.TargetNamespace + "/flow": every platform row naming anything other
// than flow reddens.
func TestEveryDraftOfTheKitIsNamedAfterTheSource(t *testing.T) {
	t.Parallel()
	drafts := []struct {
		scenario string
		served   Format
		source   string
		catalog  *aggrega.CatalogProjection
	}{
		{"node by node", testFormat(), renderSource, standardCatalog},
		{"as the platform carrying the flow", platformTestFormat(), platformSource, platformCatalog()},
		{"in the module form", moduleTestFormat(&emptyRecorder{}), moduleSource, platformCatalog()},
	}
	for _, d := range drafts {
		for _, row := range []struct {
			scenario  string
			served    Format
			name      string
			directory bool
			settings  string
			want      string
		}{
			{"a title", titled(d.served, "Support Chat"), "export.json", false, "", "imported/support-chat"},
			{"no title", d.served, "support-chat.json", false, "", "imported/support-chat"},
			{"a directory, no title", d.served, "support.chat", true, "", "imported/support-chat"},
			{"a directory, a title", titled(d.served, "Notes"), "support.chat", true, "", "imported/notes"},
			{"a title reducing to nothing, no file", titled(d.served, "  ---  "), "", false, "", "imported/flow"},
			{"a target namespace", titled(d.served, "Support Chat"), "", false, `{"target_namespace":"team-a"}`,
				"team-a/support-chat"},
		} {
			in := aggrega.ImportInput{Format: testFormatName, Bytes: []byte(d.source), Name: row.name,
				Catalog: d.catalog}
			if row.directory {
				in.Bytes, in.Sources = nil, []aggrega.ImportSource{{Name: "flows/export.json", Bytes: []byte(d.source)}}
			}
			if row.settings != "" {
				in.Settings = json.RawMessage(row.settings)
			}
			out := Import(in, row.served, (&parseRecorder{}).parse)
			var drafted []string
			for _, e := range out.Entities {
				if e.Kind == "Aggregate" && e.Mode == "draft" {
					drafted = append(drafted, e.Name)
				}
			}
			if want := []string{row.want}; !slices.Equal(drafted, want) {
				t.Errorf("%s, %s: the import drafted the Aggregate(s) %q, want %q", d.scenario, row.scenario, drafted, want)
			}
			var file string
			for _, f := range out.Files {
				if f.Path == "aggregate.yaml" {
					file = f.Content
				}
			}
			if !strings.Contains(file, "\nmetadata:\n  name: "+row.want+"\n") {
				t.Errorf("%s, %s: aggregate.yaml does not name the Aggregate %s:\n%s", d.scenario, row.scenario, row.want,
					file)
			}
		}
	}
}

// TestSeveralSourcesNamedApartFallToTheFile imports two sources through the
// platform draft, each giving its flow a title of its own at its top-level
// title, and holds the draft to no one of them: with no file named, it is named
// flow. Two sources giving the one title name it after that title.
func TestSeveralSourcesNamedApartFallToTheFile(t *testing.T) {
	t.Parallel()
	served := platformTestFormat()
	served.Title = func(tree json.RawMessage) string {
		var top struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal(tree, &top)
		return top.Title
	}
	withTitle := func(title string) []byte {
		return []byte(`{"title":` + strconv.Quote(title) + `,` + strings.TrimPrefix(platformSource, "{"))
	}
	for _, row := range []struct {
		scenario string
		a, b     string
		want     string
	}{
		{"named apart", "First", "Second", "imported/flow"},
		{"named alike", "Shared", "shared", "imported/shared"},
	} {
		sources := []aggrega.ImportSource{{Name: "a.json", Bytes: withTitle(row.a)}, {Name: "b.json", Bytes: withTitle(row.b)}}
		out := Import(aggrega.ImportInput{Format: testFormatName, Sources: sources, Catalog: platformCatalog()}, served,
			(&parseRecorder{}).parse)
		if len(out.Entities) == 0 {
			t.Fatalf("%s: the import drafted nothing: %+v", row.scenario, out.Diagnostics)
		}
		last := out.Entities[len(out.Entities)-1]
		if last.Kind != "Aggregate" || last.Name != row.want {
			t.Errorf("%s: the last entity is the %s %q, want the drafted Aggregate %q", row.scenario, last.Kind, last.Name,
				row.want)
		}
	}
}

// TestEveryDraftOfThePackageIsNamedThroughDraftName parses the package's own
// source and holds every drafted Aggregate or Release it builds — an
// EmittedEntity, written with its package, bare, or with its type elided inside
// a composite of them, whose Kind is Aggregate or Release and whose Mode is not
// reference — to a name draftName returned: the call itself, or a variable every
// assignment of which, in the function building the entity, is that call, by :=,
// by = or by a var declaration. The three drafts the package makes are required
// to be found, so a scan that read nothing fails, and the check is first run
// over a planted source holding each form, each required to be found and
// refused where it composes its own name and passed where a var declaration
// takes it from draftName.
//
// The compiler's importers are held to the same rule by a twin check,
// TestEveryImporterNamesItsDraftThroughTheKit in the compiler's
// plugins/internal/guestwire: this package is a module of its own, published
// apart, and imports nothing from the compiler's module, so the two checks are
// written once on each side.
//
// 🔴 PLANTED DEFECT: in moduleEntity (module.go), build the entity with
// Name: d.settings.TargetNamespace + "/flow": this test reddens naming module.go.
func TestEveryDraftOfThePackageIsNamedThroughDraftName(t *testing.T) {
	t.Parallel()
	const planted = `package p

func (a *b) selected() aggrega.EmittedEntity {
	return aggrega.EmittedEntity{Kind: "Aggregate", Name: a.settings.TargetNamespace + "/flow", Mode: "draft"}
}

func (a *b) bare() EmittedEntity {
	name := a.settings.TargetNamespace + "/flow"
	return EmittedEntity{Kind: "Release", Name: name, Mode: "draft"}
}

func (a *b) elided() []aggrega.EmittedEntity {
	var name = a.settings.TargetNamespace + "/flow"
	return []aggrega.EmittedEntity{{Kind: "Aggregate", Name: name, Mode: "draft"}}
}

func (a *b) declared(in aggrega.ImportInput) aggrega.EmittedEntity {
	var name = draftName(a.settings, in, nil)
	return aggrega.EmittedEntity{Kind: "Aggregate", Name: name, Mode: "draft"}
}
`
	found := draftsNamedApart(t, "planted.go", []byte(planted))
	var refused []bool
	for _, e := range found {
		refused = append(refused, e.bypass != "")
	}
	if want := []bool{true, true, true, false}; !slices.Equal(refused, want) {
		t.Fatalf("the check found %+v in the planted source; want the selected, bare and elided literals refused "+
			"and the one whose var declaration calls draftName passed", found)
	}

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob the package's source: %v", err)
	}
	sort.Strings(paths)
	var drafting []string
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, e := range draftsNamedApart(t, path, src) {
			if e.bypass != "" {
				t.Errorf("%s builds a drafted %s whose name %s; every draft is named through draftName", e.at, e.kind,
					e.bypass)
			}
			drafting = append(drafting, path)
		}
	}
	if want := []string{"emit.go", "module.go", "platform.go"}; !slices.Equal(drafting, want) {
		t.Errorf("found drafted Aggregates in %v, want one in each of %v; the scan is not reading what it checks",
			drafting, want)
	}
}

// namedDraft is one drafted Aggregate or Release a source builds: where, its
// kind, and why its name is not draftName's, empty where it is.
type namedDraft struct {
	at, kind, bypass string
}

// draftsNamedApart parses one Go source and returns every drafted Aggregate and
// Release it builds.
func draftsNamedApart(t *testing.T, at string, src []byte) []namedDraft {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, at, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", at, err)
	}
	var out []namedDraft
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		assigned := map[string][]ast.Expr{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch s := n.(type) {
			case *ast.AssignStmt:
				for i, lhs := range s.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						var rhs ast.Expr
						if len(s.Rhs) == len(s.Lhs) {
							rhs = s.Rhs[i]
						}
						assigned[id.Name] = append(assigned[id.Name], rhs)
					}
				}
			case *ast.ValueSpec:
				for i, id := range s.Names {
					var rhs ast.Expr
					if i < len(s.Values) {
						rhs = s.Values[i]
					}
					assigned[id.Name] = append(assigned[id.Name], rhs)
				}
			}
			return true
		})
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !emittedEntityType(lit.Type) {
				return true
			}
			fields := map[string]ast.Expr{}
			for _, elt := range lit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if key, ok := kv.Key.(*ast.Ident); ok {
						fields[key.Name] = kv.Value
					}
				}
			}
			kind, mode := literalText(fields["Kind"]), literalText(fields["Mode"])
			if (kind != "Aggregate" && kind != "Release") || mode == "reference" {
				return true
			}
			out = append(out, namedDraft{at: at + ":" + strconv.Itoa(fset.Position(lit.Pos()).Line), kind: kind,
				bypass: draftNameBypass(fields["Name"], assigned)})
			return true
		})
	}
	return out
}

// emittedEntityType reports whether a composite literal's type is
// EmittedEntity, written with its package or bare, or is elided, as an element
// of a composite of them writes it: an elided literal is checked as one, since
// the Kind it is given says which it is.
func emittedEntityType(typ ast.Expr) bool {
	switch t := typ.(type) {
	case nil:
		return true
	case *ast.SelectorExpr:
		return t.Sel.Name == "EmittedEntity"
	case *ast.Ident:
		return t.Name == "EmittedEntity"
	}
	return false
}

// literalText returns the value of a string literal, empty for any other
// expression.
func literalText(e ast.Expr) string {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, _ := strconv.Unquote(lit.Value)
	return s
}

// draftNameBypass says why a drafted entity's Name is not one draftName
// returned, and is empty where it is.
func draftNameBypass(name ast.Expr, assigned map[string][]ast.Expr) string {
	isDraftName := func(e ast.Expr) bool {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		id, ok := call.Fun.(*ast.Ident)
		return ok && (id.Name == "draftName" || id.Name == "DraftName")
	}
	switch e := name.(type) {
	case nil:
		return "is not set"
	case *ast.Ident:
		values := assigned[e.Name]
		if len(values) == 0 || slices.ContainsFunc(values, func(v ast.Expr) bool { return !isDraftName(v) }) {
			return "is the variable " + e.Name + ", assigned other than from draftName"
		}
		return ""
	default:
		if isDraftName(e) {
			return ""
		}
	}
	return "is composed in place rather than returned by draftName"
}
