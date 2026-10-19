// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// TestTheConventionsAnImporterDraftingItsOwnSharesAreImportsOwn holds the
// package's exports for an importer that drafts from its sources itself to what
// Import itself does: the same sources read off a request, the same parse-failed
// and no-nodes diagnostics, the same target_namespace reading, the same instance
// keys, the same Aggregate description and the same escape of a text copied into
// a drafted Aggregate.
func TestTheConventionsAnImporterDraftingItsOwnSharesAreImportsOwn(t *testing.T) {
	t.Parallel()
	primary := aggrega.ImportInput{Format: "f", Bytes: []byte("x")}
	if got := Sources(primary); !reflect.DeepEqual(got, []aggrega.ImportSource{{Name: "", Bytes: []byte("x")}}) {
		t.Errorf("Sources of a request carrying primary bytes = %+v, want them as one source with no name", got)
	}
	named := aggrega.ImportInput{Format: "f", Bytes: []byte("x"), Sources: []aggrega.ImportSource{{Name: "a", Bytes: []byte("y")}}}
	if got := Sources(named); !reflect.DeepEqual(got, named.Sources) {
		t.Errorf("Sources of a request carrying named sources = %+v, want them", got)
	}

	src := aggrega.ImportSource{Name: "flow.json"}
	if got, want := ParseFailed(src, "f", errors.New("bad")), (aggrega.Diagnostic{Severity: "halt", Code: "parse-failed",
		Message: `could not parse source "flow.json" as f: bad`, SourceRef: "flow.json"}); !reflect.DeepEqual(got, want) {
		t.Errorf("ParseFailed = %+v, want %+v", got, want)
	}
	if got, want := NoNodes(src, "f"), (aggrega.Diagnostic{Severity: "warn", Code: "no-nodes",
		Message: `source "flow.json" carried no recognisable f nodes`, SourceRef: "flow.json"}); !reflect.DeepEqual(got, want) {
		t.Errorf("NoNodes = %+v, want %+v", got, want)
	}

	for _, row := range []struct{ settings, want string }{
		{"", "imported"}, {`{}`, "imported"}, {`{"target_namespace":""}`, "imported"},
		{`{"target_namespace":"payments"}`, "payments"}, {`not json`, "imported"},
	} {
		if got := TargetNamespace(json.RawMessage(row.settings)); got != row.want {
			t.Errorf("TargetNamespace(%q) = %q, want %q", row.settings, got, row.want)
		}
	}

	used := map[string]bool{}
	var keys []string
	for _, name := range []string{"GitHub Server", "github-server", "io.github.x/y", "!!!"} {
		keys = append(keys, UniqueKey(used, name))
	}
	if want := []string{"github-server", "github-server-2", "io-github-x-y", "node"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("UniqueKey wrote %q, want %q", keys, want)
	}

	if got := DraftDescription("mcp"); got != "Draft Aggregate imported from a source in the mcp format." {
		t.Errorf("DraftDescription(mcp) = %q", got)
	}

	for _, text := range []string{`{"prompt":"{{question}}"}`, `no braces at all`, `{{{language}}}`} {
		if got, want := EscapeTemplateText(text), escapeTemplateText(text); got != want {
			t.Errorf("EscapeTemplateText(%q) = %q, want the escape a drafted flow is written with, %q", text, got, want)
		}
	}
}

// TestADecisionAndARenderingMadeNodeByNodeAreImportsOwn holds Decide and Render to
// what Import does with the same nodes: each node decided alone yields the
// decision and the entity Import yields for it, under each reuse mode and with an
// override in place, Render writes Import's files from Import's entities and
// decisions, in each render mode, and Decide says whether a row — of the arm or of
// the override — maps the node's type.
//
// 🔴 PLANTED DEFECT: in Decide (import.go), build the mapping table without the
// overrides: the overridden node is reported unmapped and decided with no table
// signal, and the case fails on it.
func TestADecisionAndARenderingMadeNodeByNodeAreImportsOwn(t *testing.T) {
	t.Parallel()
	nodes := []Node{
		{ID: "a", Type: "postgres"},
		{ID: "b", Type: "custom/image"},
		{ID: "c", Type: "unmapped"},
	}
	served := Format{
		Name:     "f",
		Syntax:   "yaml",
		Mappings: map[string]Mapping{"postgres": {Component: "core/datastore/postgres", Category: "datastore", Vendor: "postgres"}},
		Walk:     func(json.RawMessage) ([]Node, []Edge) { return nodes, nil },
	}
	catalog := &aggrega.CatalogProjection{Components: []aggrega.CatalogComponent{
		{Name: "core/datastore/postgres", Labels: map[string]string{categoryLabel: "datastore"}},
		{Name: "core/datastore/redis", Labels: map[string]string{categoryLabel: "datastore", "datastore.aggrega.ai/engine": "kv"}},
	}}
	override := `"mapping_overrides":{"f":{"custom/image":{"component":"core/datastore/redis","category":"datastore",` +
		`"traits":{"datastore.aggrega.ai/engine":"kv"},"vendor":"redis"}}}`
	for _, settings := range []string{
		`{}`,
		`{"reuse_mode":"auto-apply-above-threshold",` + override + `}`,
		`{` + override + `,"render_mode":"llm"}`,
	} {
		in := aggrega.ImportInput{Format: "f", Bytes: []byte("x"), Settings: json.RawMessage(settings), Catalog: catalog}
		whole := Import(in, served, func(string, []byte) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
		if len(whole.Decisions) != len(nodes) || len(whole.Entities) != len(nodes)+1 {
			t.Fatalf("settings %s: Import made %d decisions and %d entities", settings, len(whole.Decisions), len(whole.Entities))
		}
		for i, n := range nodes {
			decision, entity, mapped := Decide(in, served, n, n.ID)
			if !reflect.DeepEqual(decision, whole.Decisions[i]) || !reflect.DeepEqual(entity, whole.Entities[i]) {
				t.Errorf("settings %s: Decide of %s = %+v / %+v, want Import's %+v / %+v", settings, n.ID, decision, entity,
					whole.Decisions[i], whole.Entities[i])
			}
			if want := n.ID == "a" || (n.ID == "b" && settings != `{}`); mapped != want {
				t.Errorf("settings %s: Decide reports node %s mapped %v, want %v", settings, n.ID, mapped, want)
			}
		}
		if got := Render(in.Settings, whole.Entities, whole.Decisions); !reflect.DeepEqual(got, whole.Files) {
			t.Errorf("settings %s: Render wrote %+v, want Import's %+v", settings, got, whole.Files)
		}
	}
}
