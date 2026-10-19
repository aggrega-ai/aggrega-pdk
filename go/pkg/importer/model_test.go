// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"text/template"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// testModel is the test format's Model: a node whose data carries model names
// that model, one whose data carries modelWhy names none for that reason, and
// any other is of a type whose model the format does not read.
func testModel(tree json.RawMessage, node Node) (string, string) {
	_, data := testNode(tree, node.ID)
	var name, why string
	_ = json.Unmarshal(data["model"], &name)
	_ = json.Unmarshal(data["modelWhy"], &why)
	return name, why
}

// modelSource is a flow of the test's format holding five models, each mapped to
// core/ai-model/openai: one selecting gpt-4o-mini; one whose name holds a double
// brace and a colon; one naming none, for a reason it gives; one of a type whose
// model the format does not read; and one whose name holds a line break a YAML
// literal block cannot carry.
const modelSource = `{"id":"flow-m","nodes":[` +
	`{"id":"m1","type":"generic","data":{"type":"OpenAIModel","model":"gpt-4o-mini"}},` +
	`{"id":"m2","type":"generic","data":{"type":"OpenAIModel","model":"{{ .x }}: odd"}},` +
	`{"id":"m3","type":"generic","data":{"type":"OpenAIModel","modelWhy":"selects none in the test"}},` +
	`{"id":"m4","type":"generic","data":{"type":"OpenAIModel"}},` +
	`{"id":"m5","type":"generic","data":{"type":"OpenAIModel","model":"two lines"}}],` +
	`"edges":[]}` + "\n"

// referenceSettings reference every part whose best candidate is a HIGH match.
const referenceSettings = `{"reuse_mode":"auto-apply-above-threshold"}`

// TestAReferencedPartIsWrittenWithTheModelItsNodeSelects drafts modelSource with
// a format reading the model a node selects:
//
//   - referenced, against a catalog whose core/ai-model/openai declares a value
//     named model, each part is written with the model its node selects —
//     through the one writer of a value copied from a source, a double brace
//     escaped — and its decision says so; a part whose node names none the draft
//     can write is written with no value and a comment leaving values.model for
//     the author, and the import reports it, with the format's reason, the reason
//     a type it does not read gets, or the character a literal block cannot carry;
//   - drafted, under the default reuse mode — though the catalog holds a
//     Component of the drafted name declaring a value named model, as one an
//     earlier import drafted and an author kept would — or referenced as a
//     Component whose catalog entry declares no value named model, or with a
//     format reading no model, no part is asked: nothing is written and nothing
//     is reported.
//
// 🔴 PLANTED DEFECTS, each in partModel (platform.go):
//
//   - replace `if p.Model == nil || entity.Mode != "reference" {` with
//     `if p.Model == nil {`: the drafted row fails, m1 written with a model;
//   - replace `if entry == nil || !declaresValue(entry, modelValue) {` with
//     `if entry == nil {`: the row whose Component declares no model value
//     fails, m1 written with a model.
func TestAReferencedPartIsWrittenWithTheModelItsNodeSelects(t *testing.T) {
	t.Parallel()
	noModelValue := platformCatalog()
	for i, c := range noModelValue.Components {
		if c.Name == "core/ai-model/openai" {
			noModelValue.Components[i].Values = []string{"strict_response_format", "temperature"}
		}
	}
	keptDraft := platformCatalog()
	keptDraft.Components = append(keptDraft.Components, aggrega.CatalogComponent{Name: "imported/ai-model/openaimodel",
		Version: "0.1.0", Values: []string{"model"}})
	const comment = "      # values.model, the name of the model core/ai-model/openai serves, is left for the author, " +
		"since the node this part stands for "
	left := map[string]string{
		"m3": "selects none in the test",
		"m4": noModelField,
		"m5": "names a model whose name holds a line-break character U+2028, which a YAML literal block cannot carry " +
			"as written",
	}
	for _, tc := range []struct {
		name     string
		model    bool
		catalog  *aggrega.CatalogProjection
		settings string
		asked    bool
	}{
		{name: "referenced", model: true, catalog: platformCatalog(), settings: referenceSettings, asked: true},
		{name: "drafted", model: true, catalog: keptDraft},
		{name: "referenced as a Component declaring no model value", model: true, catalog: noModelValue,
			settings: referenceSettings},
		{name: "a format reading no model", catalog: platformCatalog(), settings: referenceSettings},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			format := platformTestFormat()
			if tc.model {
				format.Platform.Model = testModel
			}
			out := platformImport(t, format, tc.catalog, tc.settings, oneSource(modelSource))
			agg := out.Files[len(out.Files)-1].Content
			var reported []aggrega.Diagnostic
			for _, d := range out.Diagnostics {
				if d.Code == "model-not-named" {
					reported = append(reported, d)
				}
			}

			if !tc.asked {
				if strings.Contains(agg, "values:\n        model:") || strings.Contains(agg, "# values.model") {
					t.Errorf("a part not asked about its model is written with one or a comment:\n%s", agg)
				}
				if len(reported) > 0 {
					t.Errorf("the import reports %+v of a part not asked about its model", reported)
				}
				for _, ref := range []string{"m1", "m2", "m3", "m4", "m5"} {
					if d := decisionOf(t, out, ref); strings.Contains(d.Rationale, "values.model") {
						t.Errorf("%s's decision speaks of a model: %q", ref, d.Rationale)
					}
				}
				return
			}

			for ref, value := range map[string]string{
				"m1": "gpt-4o-mini",
				"m2": `{{"{{"}} .x }}: odd`,
			} {
				written := "    " + ref + ":\n      type: core/ai-model/openai\n      values:\n        model: |-\n          " +
					value + "\n"
				if !strings.Contains(agg, written) {
					t.Errorf("%s is not written\n%s\nthe Aggregate reads\n%s", ref, written, agg)
				}
			}
			for ref, model := range map[string]string{"m1": "gpt-4o-mini", "m2": "{{ .x }}: odd"} {
				if d := decisionOf(t, out, ref); !strings.HasSuffix(d.Rationale,
					`; values.model is written as "`+strings.ReplaceAll(model, `"`, `\"`)+`", the model the node selects`) {
					t.Errorf("%s's decision says %q", ref, d.Rationale)
				}
			}
			var want []aggrega.Diagnostic
			for _, ref := range []string{"m3", "m4", "m5"} {
				part := "    " + ref + ":\n      type: core/ai-model/openai\n" + comment + left[ref] + ".\n"
				if !strings.Contains(agg, part) {
					t.Errorf("%s is not left for the author\n%s\nthe Aggregate reads\n%s", ref, part, agg)
				}
				if d := decisionOf(t, out, ref); !strings.HasSuffix(d.Rationale,
					"; values.model is left for the author, since the node "+left[ref]) {
					t.Errorf("%s's decision says %q", ref, d.Rationale)
				}
				want = append(want, aggrega.Diagnostic{Severity: "warn", Code: "model-not-named", SourceRef: ref,
					Message: "values.model of " + ref + ", the name of the model core/ai-model/openai serves, is left for " +
						"the author, since node " + ref + " (OpenAIModel) " + left[ref] + "; a Release cannot write it"})
			}
			if !reflect.DeepEqual(reported, want) {
				t.Errorf("the import reports\n%+v\nwant\n%+v", reported, want)
			}
		})
	}
}

// TestAReasonHoldingDoubleBracesReadsBackInThePlatformsComment drafts a flow
// whose model node names no model for a reason holding "{{", which the platform
// draft writes into the comment leaving values.model for the author, and requires
// the Aggregate to run as a template — as the compiler evaluates an Aggregate's
// whole text, its comments included — and the comment to read back with the
// reason as the format gives it.
//
// 🔴 PLANTED DEFECT: in CommentLine (definition.go), return the line unescaped:
// the Aggregate no longer runs as a template, and the case fails on it.
func TestAReasonHoldingDoubleBracesReadsBackInThePlatformsComment(t *testing.T) {
	t.Parallel()
	format := platformTestFormat()
	format.Platform.Model = testModel
	source := `{"id":"flow-b","nodes":[{"id":"m3","type":"generic","data":{"type":"OpenAIModel",` +
		`"modelWhy":"names {{ a variable }} the import does not read"}}],"edges":[]}` + "\n"
	out := platformImport(t, format, platformCatalog(), referenceSettings, oneSource(source))
	agg := out.Files[len(out.Files)-1].Content
	tmpl, err := template.New("aggregate").Parse(agg)
	if err != nil {
		t.Fatalf("the drafted Aggregate does not parse as a template: %v\n%s", err, agg)
	}
	var evaluated strings.Builder
	if err := tmpl.Execute(&evaluated, nil); err != nil {
		t.Fatalf("the drafted Aggregate does not run as a template: %v\n%s", err, agg)
	}
	if want := "since the node this part stands for names {{ a variable }} the import does not read.\n"; !strings.Contains(
		evaluated.String(), want) {
		t.Errorf("the evaluated Aggregate does not say %q:\n%s", want, evaluated.String())
	}
}

// TestTheLinesATargetNeedsAreShownCommentedInThePlatform drafts a flow whose
// platform declares an attachment one target's Recipes need: where the
// platform's instance draws a connection, a comment inside it, after its type,
// shows the lines declaring that attachment, each commented out where it
// belongs; where it draws none, no line is shown.
//
// 🔴 PLANTED DEFECT: in writeTargetAttachment (platform.go), replace
// `b.WriteString(CommentLine(indent, line))` with
// `b.WriteString(indent + line + "\n")`: the wired row fails, the lines
// written uncommented.
func TestTheLinesATargetNeedsAreShownCommentedInThePlatform(t *testing.T) {
	t.Parallel()
	const lines = "    testplat:\n      type: core/ai-agent/testplat\n" +
		"      # For the compose target only, uncomment the lines below, which declare admin on testplat: the test's " +
		"reason.\n" +
		"      # features:\n" +
		"      #   admin:\n" +
		"      #     type: core/secret-value\n" +
		"      #     values:\n" +
		"      #       key: ADMIN_KEY\n" +
		"      values:\n"
	format := platformTestFormat()
	format.Platform.TargetAttachments = []TargetAttachment{{Target: "compose", Slot: "admin", Type: "core/secret-value",
		Key: "ADMIN_KEY", Why: "the test's reason"}}
	for _, tc := range []struct {
		name    string
		catalog *aggrega.CatalogProjection
		shown   bool
	}{
		{name: "an instance drawing connections", catalog: platformCatalog(), shown: true},
		{name: "an instance drawing none", catalog: platformCatalog(aggrega.CatalogSocket{Name: "unused",
			Selector: "component.aggrega.ai/category=none"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			agg := platformImport(t, format, tc.catalog, "", oneSource(platformSource)).Files
			text := agg[len(agg)-1].Content
			if got := strings.Contains(text, lines); got != tc.shown {
				t.Errorf("the lines are shown: %t, want %t; the Aggregate reads\n%s", got, tc.shown, text)
			}
			if !tc.shown && strings.Contains(text, "key: ADMIN_KEY") {
				t.Errorf("an instance drawing no connection shows the lines:\n%s", text)
			}
		})
	}
}
