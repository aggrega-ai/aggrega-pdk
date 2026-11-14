// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// TestNamedGivesTheSourceWithNoNameTheRequestsName holds Named to its three
// cases: the primary source, which carries no name of its own, takes the name the
// request gives the file it was handed on its own; a named source keeps its own
// name whatever the request carries; and a request carrying no name leaves the
// source unnamed. The bytes are untouched in every case.
func TestNamedGivesTheSourceWithNoNameTheRequestsName(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		scenario string
		in       aggrega.ImportInput
		src      aggrega.ImportSource
		want     string
	}{
		{"the primary source takes the request's name", aggrega.ImportInput{Name: "flow.json"},
			aggrega.ImportSource{Bytes: []byte("x")}, "flow.json"},
		{"a named source keeps its own", aggrega.ImportInput{Name: "flow.json"},
			aggrega.ImportSource{Name: "a.json", Bytes: []byte("x")}, "a.json"},
		{"a request naming no file leaves the source unnamed", aggrega.ImportInput{},
			aggrega.ImportSource{Bytes: []byte("x")}, ""},
	} {
		got := Named(row.in, row.src)
		if got.Name != row.want || string(got.Bytes) != "x" {
			t.Errorf("%s: Named = %+v, want the name %q and the bytes unchanged", row.scenario, got, row.want)
		}
	}
}

// TestAFileImportedOnItsOwnIsNamedInWhatTheImportReports holds the name a
// request gives the file it was handed on its own to the diagnostics Import
// reports about that file — parse-failed and no-nodes, on the node-by-node draft
// and on a platform format's — in their message and their source reference.
//
// Without Named at Import's diagnostic sites each row reads `source ""` and an
// empty reference, as a request carrying no name still does.
func TestAFileImportedOnItsOwnIsNamedInWhatTheImportReports(t *testing.T) {
	t.Parallel()
	parseFailedFlow := aggrega.Diagnostic{Severity: "halt", Code: "parse-failed",
		Message: `could not parse source "flow.json" as testflow: the source is not JSON`, SourceRef: "flow.json"}
	for _, row := range []struct {
		scenario string
		served   Format
		in       aggrega.ImportInput
		want     aggrega.Diagnostic
	}{
		{"a file that does not parse", testFormat(),
			aggrega.ImportInput{Format: testFormatName, Bytes: []byte(brokenSource), Name: "flow.json"}, parseFailedFlow},
		{"a file holding no node of the format", testFormat(),
			aggrega.ImportInput{Format: "otherflow", Bytes: []byte(renderSource), Name: "flow.json"},
			aggrega.Diagnostic{Severity: "warn", Code: "no-nodes",
				Message: `source "flow.json" carried no recognisable otherflow nodes`, SourceRef: "flow.json"}},
		{"a platform format's file that does not parse", platformTestFormat(),
			aggrega.ImportInput{Format: testFormatName, Bytes: []byte(brokenSource), Name: "flow.json",
				Catalog: platformCatalog()}, parseFailedFlow},
	} {
		out := Import(row.in, row.served, (&parseRecorder{}).parse)
		if want := []aggrega.Diagnostic{row.want}; !reflect.DeepEqual(out.Diagnostics, want) {
			t.Errorf("%s: the import reported %+v, want %+v", row.scenario, out.Diagnostics, want)
		}
	}
}

// TestTheRequestsNameMovesOnlyTheDraftsName holds the name to what an import
// reports and to the drafted Aggregate's name: the same file imported with and
// without the request's name drafts the same entities, decisions and files, on
// the node-by-node draft and on a platform format's, but for the name of the
// Aggregate and of the Release deploying it, which a format reading no title of
// its own takes from the file's stem —
// a source is drafted under its own name and the primary source carries none.
func TestTheRequestsNameMovesOnlyTheDraftsName(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		scenario string
		served   Format
		in       aggrega.ImportInput
	}{
		{"node by node", testFormat(), aggrega.ImportInput{Format: testFormatName, Bytes: []byte(renderSource),
			Catalog: standardCatalog}},
		{"as the platform carrying the flow", platformTestFormat(), aggrega.ImportInput{Format: testFormatName,
			Bytes: []byte(platformSource), Catalog: platformCatalog()}},
	} {
		unnamed := Import(row.in, row.served, (&parseRecorder{}).parse)
		named := row.in
		named.Name = "support-chat.json"
		got := Import(named, row.served, (&parseRecorder{}).parse)
		if len(unnamed.Entities) == 0 {
			t.Fatalf("%s: the import drafted nothing, so the comparison holds nothing", row.scenario)
		}
		// The unnamed import, its Aggregate and the Release deploying it renamed as
		// the file's stem names them.
		const unnamedLine, namedLine = "\n  name: imported/flow\n", "\n  name: imported/support-chat\n"
		const unnamedRef, namedRef = "\n  aggregate: imported/flow\n", "\n  aggregate: imported/support-chat\n"
		rename := func(body string) string {
			return strings.Replace(strings.Replace(body, unnamedLine, namedLine, 1), unnamedRef, namedRef, 1)
		}
		want := unnamed
		want.Entities = append([]aggrega.EmittedEntity(nil), unnamed.Entities...)
		for i, e := range want.Entities {
			if (e.Kind == "Aggregate" || e.Kind == "Release") && e.Mode == "draft" {
				want.Entities[i].Name = "imported/support-chat"
				want.Entities[i].Body = rename(e.Body)
			}
		}
		want.Files = append([]aggrega.ScaffoldFile(nil), unnamed.Files...)
		for i, f := range want.Files {
			if f.Path == "aggregate.yaml" || f.Path == ReleaseFileName {
				want.Files[i].Content = rename(f.Content)
			}
		}
		if !reflect.DeepEqual(got.Entities, want.Entities) || !reflect.DeepEqual(got.Decisions, want.Decisions) ||
			!reflect.DeepEqual(got.Files, want.Files) {
			t.Errorf("%s: naming the file moved more than the Aggregate's name:\nnamed %+v\nwant  %+v", row.scenario,
				got, want)
		}
	}
}
