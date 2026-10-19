// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"text/template"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// TestTheConventionsAnImporterDraftingItsOwnSharesAreImportsOwn holds the
// package's exports for an importer that drafts from its sources itself to what
// Import itself does: the same sources read off a request, the same parse-failed
// and no-nodes diagnostics, the same target_namespace reading, the same instance
// keys and the same Aggregate description; and the escape of a text copied into
// a drafted Aggregate written as a raw-string action.
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
		if got, want := EscapeTemplateText(text), strings.ReplaceAll(text, "{{", "{{`{{`}}"); got != want {
			t.Errorf("EscapeTemplateText(%q) = %q, want every opening pair written as a raw-string action, %q", text,
				got, want)
		}
	}
}

// TestCommentLineReadsBackAsWrittenOnceEvaluated writes comment lines holding
// "{{" and requires each to be one line at its indent that, evaluated as a
// template as the compiler evaluates an Aggregate, reads back as the line written
// as it is; a line holding none is written as it is.
//
// 🔴 PLANTED DEFECT: in CommentLine (definition.go), return the line unescaped:
// every row holding "{{" fails to parse as a template.
func TestCommentLineReadsBackAsWrittenOnceEvaluated(t *testing.T) {
	t.Parallel()
	for _, text := range []string{`the model {{ x }}.`, `ends with {{`, `{{{a}}} and {{b}}`, `no braces at all`} {
		t.Run(text, func(t *testing.T) {
			line := CommentLine("    ", text)
			if !strings.HasPrefix(line, "    # ") || strings.Count(line, "\n") != 1 || !strings.HasSuffix(line, "\n") {
				t.Fatalf("CommentLine wrote %q, want one comment line at the indent", line)
			}
			tmpl, err := template.New("aggregate").Parse(line)
			if err != nil {
				t.Fatalf("the line %q does not parse as a template: %v", line, err)
			}
			var b strings.Builder
			if err := tmpl.Execute(&b, nil); err != nil {
				t.Fatalf("the line %q does not run as a template: %v", line, err)
			}
			if want := "    # " + text + "\n"; b.String() != want {
				t.Errorf("the line reads back as %q, want %q", b.String(), want)
			}
		})
	}
}

// TestEscapeTemplateTextReadsBackInsideADoubleQuotedScalar writes texts holding
// double braces, double quotes and backslashes through EscapeTemplateText and
// then as a double-quoted scalar, evaluates the result as a Go template — as an
// Aggregate's text is evaluated before it is read as YAML — and reads the scalar
// back: each reads back as the text it was. JSON's string escapes are a subset of
// a YAML double-quoted scalar's, so the scalar is read back as JSON.
//
// 🔴 PLANTED DEFECT: make EscapeTemplateText return escapeTemplateText(text), the
// double-quoted action: the scalar escapes its quotes with backslashes the
// template engine refuses, and every row holding "{{" fails.
func TestEscapeTemplateTextReadsBackInsideADoubleQuotedScalar(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		`tutors {{ students }} in code`,
		`{{{language}}} and a "quote"`,
		`ends with {{`,
		`a back\slash and {{x}}`,
		`no braces at all`,
	} {
		t.Run(text, func(t *testing.T) {
			quoted, err := json.Marshal(EscapeTemplateText(text))
			if err != nil {
				t.Fatalf("quote the escaped text: %v", err)
			}
			tmpl, err := template.New("aggregate").Parse(string(quoted))
			if err != nil {
				t.Fatalf("the quoted escaped text %s does not parse as a template: %v", quoted, err)
			}
			var b strings.Builder
			if err := tmpl.Execute(&b, nil); err != nil {
				t.Fatalf("the quoted escaped text %s does not run as a template: %v", quoted, err)
			}
			var got string
			if err := json.Unmarshal([]byte(b.String()), &got); err != nil {
				t.Fatalf("the evaluated scalar %s does not read back: %v", b.String(), err)
			}
			if got != text {
				t.Errorf("the scalar reads back as %q, want %q", got, text)
			}
		})
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
