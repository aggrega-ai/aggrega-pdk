// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// TestIsVariableReference tells a value that is one variable reference and
// nothing else — the bare name of a variable, or ${env:NAME} or ${input:id} —
// from one holding a value: a reference carrying a default, an alternative, a
// message or an assignment holds the text after its operator, with or without a
// colon before it; another form inside ${…} holds its text; and text beside a
// reference holds that text.
//
// 🔴 PLANTED DEFECT: in IsVariableReference (credentials.go), replace
// `return variableName(inner)` with
// `return !strings.ContainsAny(inner, "${}\n\r") && !strings.Contains(inner, ":-")`
// — the grammar refusing only a colon and a hyphen: the rows for ${TOKEN-…},
// ${TOKEN?…}, ${TOKEN:?…}, ${TOKEN:+…}, ${TOKEN+…}, ${TOKEN=…}, ${TOKEN:=…} and
// the forms no client defines read as references, and fail.
func TestIsVariableReference(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		in   string
		want bool
	}{
		{"${TOKEN}", true}, {"${env:TOKEN}", true}, {"${input:api-key}", true}, {"${workspaceFolder}", true},
		{"${a}", true}, {"${_TOKEN_2}", true}, {"${userHome}", true}, {"${env:api-key}", true},
		{"${input:perplexity_key-2}", true},
		{"${TOKEN:-default}", false}, {"Bearer ${TOKEN}", false}, {"--key=${TOKEN}", false}, {"${A}${B}", false},
		{"${}", false}, {"$TOKEN", false}, {"plain", false}, {"", false}, {"${TOKEN", false}, {"${TO\nKEN}", false},
		{" ${TOKEN}", false},
		{"${TOKEN-sk-planted}", false}, {"${TOKEN?sk-planted}", false}, {"${TOKEN:?sk-planted}", false},
		{"${TOKEN:+sk-planted}", false}, {"${TOKEN+sk-planted}", false}, {"${TOKEN=sk-planted}", false},
		{"${TOKEN:=sk-planted}", false}, {"${TOKEN-}", false}, {"${1TOKEN}", false}, {"${TOK EN}", false},
		{"${env:}", false}, {"${input:}", false}, {"${env:TOKEN:-sk-planted}", false}, {"${input:a=b}", false},
		{"${config:editor.fontSize}", false}, {"${command:pick}", false}, {"${workspaceFolder:Client}", false},
		{"${user_config.KEY}", false}, {"${/}", false},
	} {
		if got := IsVariableReference(row.in); got != row.want {
			t.Errorf("IsVariableReference(%q) = %v, want %v", row.in, got, row.want)
		}
	}
}

// referenceCarriedAs is the Aggregate drafted for platformSourceLine with
// plantedFields written in it, as the platform draft carries it.
func referenceCarriedAs(apiKey, arg0, arg1, auth string) string {
	agg := renderedPlatformAggregate()
	agg = strings.Replace(agg, `"secretValues":{"api_key":""}`, `"secretValues":{"api_key":"`+apiKey+`"}`, 1)
	return strings.Replace(agg, `{"type":"Finder"}`, `{"type":"Finder","args":["`+arg0+`","`+arg1+`"],`+
		`"headers":{"Authorization":"`+auth+`","Accept":""}}`, 1)
}

// TestAValueThatIsWhollyAVariableReferenceIsLeftAsWritten imports, through the
// platform draft, a flow whose located values of every class are variable
// references — the field the test's format marks as a credential holding
// ${API_KEY}, the first argument a server is started with ${TOKEN}, the header
// the format marks as none ${env:AUTH} — beside a second argument holding a
// key-shaped value, under the empty_credentials setting absent and at each value
// it takes:
//
//   - every reference is carried as written, in the flow and nowhere else, and
//     named in no warning, under every value of the setting;
//   - the second argument is carried and named in a credential-carried warning
//     by default, and written empty and named in a credential-emptied warning
//     under marked and all, as any value of its class is.
//
// A second flow holds, in the same places, a reference carrying a default and
// text beside a reference, which hold values: by default each is carried and
// named in a credential-carried warning, and under all each is written empty. A
// third holds references carrying a key-shaped default, message, alternative
// and assignment after an operator with no colon before it — ${API_KEY-…},
// ${TOKEN?…}, ${TOKEN:+…}, ${AUTH=…} — which hold that text as a default after
// :- does: each is carried and named in a credential-carried warning by
// default, and written empty where the setting asks for its class.
//
// 🔴 PLANTED DEFECT: in carryCredentials (credentials.go), replace
// `case references && wholeReference(tree, c):` with `case false:` — the test
// removed: every reference is named in a credential-carried warning by default
// and written empty under marked and all, and every row of the first flow fails.
// A second: in IsVariableReference, replace `return variableName(inner)` with
// `return !strings.ContainsAny(inner, "${}\n\r") && !strings.Contains(inner, ":-")`
// — the operators with no colon read as references: every row of the third
// flow fails, each value left as written and named in no warning.
func TestAValueThatIsWhollyAVariableReferenceIsLeftAsWritten(t *testing.T) {
	t.Parallel()
	references := []string{"${API_KEY}", "${TOKEN}", "${env:AUTH}"}
	source := plantedFields(platformSourceLine, "${API_KEY}", "${TOKEN}", "--api-key="+plantedServer,
		"${env:AUTH}") + "\n"
	for _, needle := range append([]string{plantedServer}, references...) {
		if !strings.Contains(source, needle) {
			t.Fatalf("the needle: the search does not find %s in the source, so it finds nothing", needle)
		}
	}
	asWritten := referenceCarriedAs("${API_KEY}", "${TOKEN}", "--api-key="+plantedServer, "${env:AUTH}")
	argumentEmptied := referenceCarriedAs("${API_KEY}", "${TOKEN}", "", "${env:AUTH}")
	for _, tc := range []struct {
		name, settings, aggregate string
		diagnostics               []aggrega.Diagnostic
	}{
		{name: "absent", aggregate: asWritten,
			diagnostics: []aggrega.Diagnostic{carriedWarning("finder", "args.1", serverWhy, "marked or all")}},
		{name: "none", settings: `{"empty_credentials":"none"}`, aggregate: asWritten,
			diagnostics: []aggrega.Diagnostic{carriedWarning("finder", "args.1", serverWhy, "marked or all")}},
		{name: "marked", settings: `{"empty_credentials":"marked"}`, aggregate: argumentEmptied,
			diagnostics: []aggrega.Diagnostic{emptiedWarning("finder", "args.1", testArgumentDetail)}},
		{name: "all", settings: `{"empty_credentials":"all"}`, aggregate: argumentEmptied,
			diagnostics: []aggrega.Diagnostic{emptiedWarning("finder", "args.1", testArgumentDetail)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := platformImport(t, platformTestFormat(), platformCatalog(), tc.settings, oneSource(source))
			if agg := out.Files[len(out.Files)-1].Content; agg != tc.aggregate {
				t.Errorf("the Aggregate is not the source carried as the setting asks:\n%s\nwant\n%s", agg, tc.aggregate)
			}
			if !reflect.DeepEqual(out.Diagnostics, tc.diagnostics) {
				t.Errorf("the import raised %+v, want %+v", out.Diagnostics, tc.diagnostics)
			}
			noPlantedValueOutsideTheFlow(t, out, append([]string{plantedServer}, references...))
		})
	}

	valued := plantedFields(platformSourceLine, plantedMarked, "${TOKEN:-"+plantedServer+"}", "--key=${TOKEN}",
		"Bearer ${TOKEN}") + "\n"
	for _, tc := range []struct {
		name, settings, aggregate string
		diagnostics               []aggrega.Diagnostic
	}{
		{name: "values beside a reference, absent",
			aggregate: referenceCarriedAs(plantedMarked, "${TOKEN:-"+plantedServer+"}", "--key=${TOKEN}", "Bearer ${TOKEN}"),
			diagnostics: []aggrega.Diagnostic{
				carriedWarning("model-1", "api_key", markedWhy, "marked or all"),
				carriedWarning("finder", "args.0", serverWhy, "marked or all"),
				carriedWarning("finder", "args.1", serverWhy, "marked or all"),
				carriedWarning("finder", "headers.Authorization", unmarkedWhy, "all"),
			}},
		{name: "values beside a reference, all", settings: `{"empty_credentials":"all"}`,
			aggregate: referenceCarriedAs("", "", "", ""),
			diagnostics: []aggrega.Diagnostic{
				emptiedWarning("model-1", "api_key", ""),
				emptiedWarning("finder", "args.0", testArgumentDetail),
				emptiedWarning("finder", "args.1", testArgumentDetail),
				emptiedWarning("finder", "headers.Authorization", testHeaderDetail),
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := platformImport(t, platformTestFormat(), platformCatalog(), tc.settings, oneSource(valued))
			if agg := out.Files[len(out.Files)-1].Content; agg != tc.aggregate {
				t.Errorf("the Aggregate is not the source carried as the setting asks:\n%s\nwant\n%s", agg, tc.aggregate)
			}
			if !reflect.DeepEqual(out.Diagnostics, tc.diagnostics) {
				t.Errorf("the import raised %+v, want %+v", out.Diagnostics, tc.diagnostics)
			}
			noPlantedValueOutsideTheFlow(t, out, []string{plantedMarked, plantedServer})
		})
	}

	operators := []string{"${API_KEY-" + plantedMarked + "}", "${TOKEN?" + plantedServer + "}",
		"${TOKEN:+" + plantedServer + "}", "${AUTH=" + plantedUnmarked + "}"}
	operated := plantedFields(platformSourceLine, operators[0], operators[1], operators[2], operators[3]) + "\n"
	for _, needle := range operators {
		if !strings.Contains(operated, needle) {
			t.Fatalf("the needle: the search does not find %s in the source, so it finds nothing", needle)
		}
	}
	for _, tc := range []struct {
		name, settings, aggregate string
		diagnostics               []aggrega.Diagnostic
	}{
		{name: "an operator with no colon, absent",
			aggregate: referenceCarriedAs(operators[0], operators[1], operators[2], operators[3]),
			diagnostics: []aggrega.Diagnostic{
				carriedWarning("model-1", "api_key", markedWhy, "marked or all"),
				carriedWarning("finder", "args.0", serverWhy, "marked or all"),
				carriedWarning("finder", "args.1", serverWhy, "marked or all"),
				carriedWarning("finder", "headers.Authorization", unmarkedWhy, "all"),
			}},
		{name: "an operator with no colon, marked", settings: `{"empty_credentials":"marked"}`,
			aggregate: referenceCarriedAs("", "", "", operators[3]),
			diagnostics: []aggrega.Diagnostic{
				emptiedWarning("model-1", "api_key", ""),
				emptiedWarning("finder", "args.0", testArgumentDetail),
				emptiedWarning("finder", "args.1", testArgumentDetail),
				carriedWarning("finder", "headers.Authorization", unmarkedWhy, "all"),
			}},
		{name: "an operator with no colon, all", settings: `{"empty_credentials":"all"}`,
			aggregate: referenceCarriedAs("", "", "", ""),
			diagnostics: []aggrega.Diagnostic{
				emptiedWarning("model-1", "api_key", ""),
				emptiedWarning("finder", "args.0", testArgumentDetail),
				emptiedWarning("finder", "args.1", testArgumentDetail),
				emptiedWarning("finder", "headers.Authorization", testHeaderDetail),
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := platformImport(t, platformTestFormat(), platformCatalog(), tc.settings, oneSource(operated))
			if agg := out.Files[len(out.Files)-1].Content; agg != tc.aggregate {
				t.Errorf("the Aggregate is not the source carried as the setting asks:\n%s\nwant\n%s", agg, tc.aggregate)
			}
			if !reflect.DeepEqual(out.Diagnostics, tc.diagnostics) {
				t.Errorf("the import raised %+v, want %+v", out.Diagnostics, tc.diagnostics)
			}
			noPlantedValueOutsideTheFlow(t, out, []string{plantedMarked, plantedServer, plantedUnmarked})
		})
	}
}

// TestADraftingImporterHandsOnlyTheValuesItsGrammarSaysHoldOne hands
// CarryCredentials the drafted document holding a variable reference in each
// located value but one, at empty_credentials marked: the decision judges each
// value by its class alone, so each it is handed is written empty and reported,
// the references included — which is why an importer drafting values of its own
// reads its format's references itself and hands none of them, as the MCP
// importer does through IsVariableReference and the Compose importer through
// Compose's own interpolation grammar.
func TestADraftingImporterHandsOnlyTheValuesItsGrammarSaysHoldOne(t *testing.T) {
	t.Parallel()
	doc := plantedFields(platformSourceLine, "${API_KEY}", "${TOKEN}", "--api-key="+plantedServer, "${env:AUTH}")
	located := testCredentials(json.RawMessage(doc))
	written, diags, err := CarryCredentials(json.RawMessage(`{"empty_credentials":"marked"}`), []byte(doc),
		"in the Release the draft writes", located)
	if err != nil {
		t.Fatalf("the decision refused the document: %v", err)
	}
	if want := plantedFields(platformSourceLine, "", "", "", "${env:AUTH}"); string(written) != want {
		t.Errorf("the drafted document reads\n%s\nwant\n%s", written, want)
	}
	var codes []string
	for _, d := range diags {
		codes = append(codes, d.Code+" "+d.SourceRef)
	}
	if want := []string{"credential-emptied model-1", "credential-emptied finder", "credential-emptied finder",
		"credential-carried finder"}; !reflect.DeepEqual(codes, want) {
		t.Errorf("the decision raised %v, want %v", codes, want)
	}
}
