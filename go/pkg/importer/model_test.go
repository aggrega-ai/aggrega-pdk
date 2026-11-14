// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"text/template"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
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
//   - drafted, under the hints-only reuse mode — though the catalog holds a
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
		{name: "drafted", model: true, catalog: keptDraft, settings: hintsOnly},
		{name: "referenced under the default reuse mode", model: true, catalog: platformCatalog(), asked: true},
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
			agg := aggregateFile(out)
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
	agg := aggregateFile(out)
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

// TestTheLinesATargetNeedsAreWrittenForTheImportsTarget drafts a flow whose
// platform declares an attachment one target's Recipes need, for each target the
// request can carry. Where the platform's instance draws a connection: an import
// naming no target shows the lines declaring that attachment in a comment inside
// the instance, after its type, each commented out where it belongs; an import
// for that target declares them there, under features, beneath a comment saying
// why; an import for another target writes none of them. Where the instance
// draws no connection, no line is written for any target.
//
// 🔴 PLANTED DEFECTS, each in platformDraft.entity (platform.go):
//
//   - replace `case a.Target == in.Target:` with `case false:`: the row for the
//     attachment's own target fails, the lines left out;
//   - replace `case in.Target == "":` with `case true:`: the rows for a target
//     fail, the lines written commented whatever the target.
func TestTheLinesATargetNeedsAreWrittenForTheImportsTarget(t *testing.T) {
	t.Parallel()
	const head = "    testplat:\n      type: core/ai-agent/testplat\n"
	const commented = head +
		"      # For the compose target only, uncomment the lines below, which declare admin on testplat: the test's " +
		"reason.\n" +
		"      # features:\n" +
		"      #   admin:\n" +
		"      #     type: core/secret-value\n" +
		"      #     values:\n" +
		"      #       key: ADMIN_KEY\n" +
		"      values:\n"
	const declared = head +
		"      features:\n" +
		"        # For the compose target, the lines below declare admin on testplat: the test's reason.\n" +
		"        admin:\n" +
		"          type: core/secret-value\n" +
		"          values:\n" +
		"            key: ADMIN_KEY\n" +
		"      values:\n"
	const none = head + "      values:\n"
	format := platformTestFormat()
	format.Platform.TargetAttachments = []TargetAttachment{{Target: "compose", Slot: "admin", Type: "core/secret-value",
		Key: "ADMIN_KEY", Why: "the test's reason"}}
	unwired := platformCatalog(aggrega.CatalogSocket{Name: "unused", Selector: "component.aggrega.ai/category=none"})
	for _, tc := range []struct {
		name, target, want string
		catalog            *aggrega.CatalogProjection
	}{
		{name: "no target, an instance drawing connections", want: commented, catalog: platformCatalog()},
		{name: "the attachment's target", target: "compose", want: declared, catalog: platformCatalog()},
		{name: "another target", target: "helm", want: none, catalog: platformCatalog()},
		{name: "no target, an instance drawing none", want: none, catalog: unwired},
		{name: "the attachment's target, an instance drawing none", target: "compose", want: none, catalog: unwired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := aggrega.ImportInput{Format: testFormatName, Bytes: []byte(platformSource), Catalog: tc.catalog,
				Target: tc.target}
			text := aggregateFile(Import(in, format, (&parseRecorder{}).parse))
			if !strings.Contains(text, tc.want) {
				t.Errorf("the instance does not read\n%s\nthe Aggregate reads\n%s", tc.want, text)
			}
			if tc.want == none && strings.Contains(text, "ADMIN_KEY") {
				t.Errorf("the instance writes a line of the attachment:\n%s", text)
			}
		})
	}
}
