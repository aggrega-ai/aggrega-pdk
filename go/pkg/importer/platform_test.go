// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// testPlatformName is the platform Component of the test's own format.
const testPlatformName = "core/ai-agent/testplat"

// platformTestFormat is the test's own format, describing a platform of the
// test's own: its flow is carried in flow_definition and named by flow_id, the
// source's top-level id is its own identifier, every member of a node's
// data.secretValues is a credential, and so is every element of its data.args,
// the arguments a server is started with, and a socket's selector is read by
// testMatch.
func platformTestFormat() Format {
	f := testFormat()
	f.Platform = &Platform{
		Component:       testPlatformName,
		DefinitionValue: "flow_definition",
		IdentifierValue: "flow_id",
		Identifier:      testFlowID,
		Credentials:     testCredentials,
		Match:           testMatch,
		LeftForRelease:  []string{"infra.image, the image running the platform", "the admin slot, the administrator's password"},
	}
	return f
}

func testFlowID(tree json.RawMessage) string {
	var top struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(tree, &top)
	return top.ID
}

// testArgumentDetail says why the test's format writes an argument empty.
const testArgumentDetail = "an argument a server is started with can carry a credential"

func testCredentials(tree json.RawMessage) []Credential {
	var top struct {
		Nodes []struct {
			ID   string `json:"id"`
			Data struct {
				SecretValues json.RawMessage `json:"secretValues"`
				Args         json.RawMessage `json:"args"`
			} `json:"data"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(tree, &top); err != nil {
		return nil
	}
	var out []Credential
	for i, n := range top.Nodes {
		for _, e := range ObjectEntries(n.Data.SecretValues) {
			out = append(out, Credential{Node: n.ID, Field: e.Key,
				Pointer: Pointer("nodes", strconv.Itoa(i), "data", "secretValues", e.Key)})
		}
		var args []json.RawMessage
		_ = json.Unmarshal(n.Data.Args, &args)
		for j := range args {
			out = append(out, Credential{Node: n.ID, Field: "args." + strconv.Itoa(j),
				Pointer: Pointer("nodes", strconv.Itoa(i), "data", "args", strconv.Itoa(j)), Detail: testArgumentDetail})
		}
	}
	return out
}

// testMatch is the test's own selector reading: comma-separated terms, each
// key=value (the label holds that value) or key (the label is present), all of
// which must hold. A term with an empty key does not read.
func testMatch(selector string, labels map[string]string) (bool, error) {
	for _, term := range strings.Split(selector, ",") {
		key, value, equals := strings.Cut(strings.TrimSpace(term), "=")
		if key == "" {
			return false, errors.New("a term names no key")
		}
		got, present := labels[key]
		if !present || (equals && got != value) {
			return false, nil
		}
	}
	return true, nil
}

// testPlatformEntry is the catalog's entry for the test's platform: a model
// socket admitting a generative model, and a tools socket admitting a read-only
// tool, in that order.
var testPlatformEntry = aggrega.CatalogComponent{
	Name: testPlatformName, Version: "1.0.0",
	Labels: map[string]string{"component.aggrega.ai/type": "server", "component.aggrega.ai/category": "ai-agent"},
	Values: []string{"flow_definition", "flow_id"},
	Sockets: []aggrega.CatalogSocket{
		{Name: "model", Selector: "component.aggrega.ai/category=ai-model,model.aggrega.ai/capability=generative"},
		{Name: "tools", Selector: "component.aggrega.ai/category=ai-tool,tool.aggrega.ai/impact=read-only"},
	},
}

// platformCatalog is standardCatalog with the platform's entry among it, in name
// order, carrying the sockets given — testPlatformEntry's where none are.
func platformCatalog(sockets ...aggrega.CatalogSocket) *aggrega.CatalogProjection {
	entry := testPlatformEntry
	if len(sockets) > 0 {
		entry.Sockets = sockets
	}
	view := &aggrega.CatalogProjection{}
	for _, c := range standardCatalog.Components {
		view.Components = append(view.Components, c)
		if c.Name == "core/ai-agent/langflow" {
			view.Components = append(view.Components, entry)
		}
	}
	return view
}

// platformImport runs Import over the sources given of the test's platform
// format against the catalog view given, with the settings document given (""
// for none). One source is handed as the primary bytes, several as named sources.
func platformImport(t *testing.T, format Format, catalog *aggrega.CatalogProjection, settings string,
	sources ...aggrega.ImportSource) aggrega.ImportOutput {
	t.Helper()
	in := aggrega.ImportInput{Format: testFormatName, Catalog: catalog}
	if len(sources) == 1 {
		in.Bytes = sources[0].Bytes
	} else {
		in.Sources = sources
	}
	if settings != "" {
		in.Settings = json.RawMessage(settings)
	}
	return Import(in, format, (&parseRecorder{}).parse)
}

func oneSource(body string) aggrega.ImportSource {
	return aggrega.ImportSource{Bytes: []byte(body)}
}

// platformSourceLine is a flow of four nodes and a note: a mapped model holding
// a credential field whose value is empty, a mapped tool, a custom node whose
// prompt writes a variable in double braces, and a chat output; and the edges
// between them. Its top-level id is the flow's own identifier.
const platformSourceLine = `{"id":"flow-7","nodes":[` +
	`{"id":"model-1","type":"generic","data":{"type":"OpenAIModel","fields":["model_name","temperature"],"secrets":["api_key"],"secretValues":{"api_key":""}}},` +
	`{"id":"finder","type":"generic","data":{"type":"Finder"}},` +
	`{"id":"Custom Logic","type":"generic","data":{"type":"CustomPythonComponent","fields":["code"],"prompt":"Answer {{question}}"}},` +
	`{"id":"out","type":"generic","data":{"type":"ChatOutput"}},` +
	`{"id":"n1","type":"note","data":{"type":"Note"}}],` +
	`"edges":[{"source":"model-1","target":"Custom Logic"},{"source":"finder","target":"Custom Logic"},{"source":"Custom Logic","target":"out"}]}`

const platformSource = platformSourceLine + "\n"

// renderedFinder is the Component drafted for the finder node.
const renderedFinder = `# resembles core/ai-tool/retrieval (score 0.7000); consider referencing it
apiVersion: aggrega.ai/v1alpha1
kind: Component
metadata:
  name: imported/ai-tool/finder
  version: 0.1.0
  labels:
    component.aggrega.ai/type: function
    component.aggrega.ai/category: ai-tool
spec: {}
`

// renderedPlatformAggregate is the Aggregate drafted for platformSource under the
// default settings, its definition written with the double braces escaped by the
// test's own spelling of the escape.
func renderedPlatformAggregate() string {
	escaped := strings.Replace(platformSourceLine, "{{question}}", `{{"{{"}}question}}`, 1)
	return `apiVersion: aggrega.ai/v1alpha1
kind: Aggregate
metadata:
  name: imported/flow
  version: 0.1.0
  description: Draft aggregate imported from a testflow source.
spec:
  components:
    # Left for the Release, since the source cannot supply it:
    #   - infra.image, the image running the platform
    #   - the admin slot, the administrator's password
    #   - the credential each connection under uses presents to the part it reaches, attached on that connection
    testplat:
      type: core/ai-agent/testplat
      values:
        flow_id: 'flow-7'
        flow_definition: |
          ` + escaped + `
      uses:
        model:
          model-1:
            target: model-1
        tools:
          finder:
            target: finder
    # resembles core/ai-model/openai (score 0.9200); reference it by setting type: core/ai-model/openai
    model-1:
      type: imported/ai-model/openaimodel
    # resembles core/ai-tool/retrieval (score 0.7000); consider referencing it
    finder:
      type: imported/ai-tool/finder
`
}

// decisionOf returns the decision recorded for the node referenced as ref.
func decisionOf(t *testing.T, out aggrega.ImportOutput, ref string) aggrega.ImportDecision {
	t.Helper()
	for _, d := range out.Decisions {
		if d.SourceNode.ID == ref {
			return d
		}
	}
	t.Fatalf("no decision was recorded for %s: %+v", ref, out.Decisions)
	return aggrega.ImportDecision{}
}

// TestThePlatformLayoutDraftsThePlatformCarryingTheFlow pins, byte for byte, the
// default draft of a format describing a platform: the platform's instance
// carrying the flow — its own identifier, its definition with the double braces
// escaped — and the two parts the platform's sockets take wired to them in the
// sockets' order, each drafted as the node-by-node draft drafts it; the custom
// node and the chat output stay in the flow and become no Component. Every node
// keeps a decision: a wired part says where it is wired, and a node the flow keeps
// chooses nothing, offers no swap and says why it is kept. An attachment writing
// layout: platform drafts exactly the same.
func TestThePlatformLayoutDraftsThePlatformCarryingTheFlow(t *testing.T) {
	t.Parallel()
	out := platformImport(t, platformTestFormat(), platformCatalog(), "", oneSource(platformSource))

	want := []aggrega.ScaffoldFile{
		{Path: "components/imported-ai-model-openaimodel.yaml", Content: renderedModel},
		{Path: "components/imported-ai-tool-finder.yaml", Content: renderedFinder},
		{Path: "aggregate.yaml", Content: renderedPlatformAggregate()},
	}
	if !reflect.DeepEqual(out.Files, want) {
		t.Errorf("the platform draft wrote\n%+v\nwant\n%+v", out.Files, want)
	}
	if len(out.Diagnostics) != 0 {
		t.Errorf("the import raised %+v; this source reads with none", out.Diagnostics)
	}

	var kinds []string
	for _, e := range out.Entities {
		kinds = append(kinds, e.Kind+" "+e.Mode+" "+e.Name)
	}
	wantKinds := []string{
		"Component reference " + testPlatformName,
		"Component draft imported/ai-model/openaimodel",
		"Component draft imported/ai-tool/finder",
		"Aggregate draft imported/flow",
	}
	if !reflect.DeepEqual(kinds, wantKinds) {
		t.Errorf("the import emitted %v, want %v", kinds, wantKinds)
	}

	for _, tc := range []struct {
		ref, rationale, hint string
		chosen               *aggrega.DecisionChoice
		candidates           bool
	}{
		{ref: "model-1", candidates: true,
			chosen: &aggrega.DecisionChoice{Target: "imported/ai-model/openaimodel", Mode: "draft", Tier: tierHigh},
			rationale: "drafted a new component; core/ai-model/openai is a strong match at score 0.9200 but hints-only mode " +
				"references nothing; wired to the model socket of core/ai-agent/testplat as model-1",
			hint: "resembles core/ai-model/openai (score 0.9200); reference it by setting type: core/ai-model/openai"},
		{ref: "finder", candidates: true,
			chosen: &aggrega.DecisionChoice{Target: "imported/ai-tool/finder", Mode: "draft", Tier: tierMedium},
			rationale: "drafted a new component; core/ai-tool/retrieval is a partial match at score 0.7000; wired to the " +
				"tools socket of core/ai-agent/testplat as finder",
			hint: "resembles core/ai-tool/retrieval (score 0.7000); consider referencing it"},
		{ref: "Custom Logic",
			rationale: "kept in the flow core/ai-agent/testplat carries, with no Component of its own: no mapping row " +
				"names its type and no Component of the workspace's catalog resembles it"},
		{ref: "out", candidates: true,
			rationale: "kept in the flow core/ai-agent/testplat carries, with no Component of its own: no mapping row " +
				"names its type and its closest candidate core/ai-model/anthropic scored only 0.2000 (LOW)"},
	} {
		t.Run(tc.ref, func(t *testing.T) {
			d := decisionOf(t, out, tc.ref)
			if !reflect.DeepEqual(d.Chosen, tc.chosen) {
				t.Errorf("the choice is %+v, want %+v", d.Chosen, tc.chosen)
			}
			if d.Rationale != tc.rationale {
				t.Errorf("the rationale reads\n  %q\nwant\n  %q", d.Rationale, tc.rationale)
			}
			if d.SwapHint != tc.hint {
				t.Errorf("the hint reads %q, want %q", d.SwapHint, tc.hint)
			}
			if (len(d.Candidates) > 0) != tc.candidates {
				t.Errorf("the decision carries the candidates %+v; want candidates %v", d.Candidates, tc.candidates)
			}
		})
	}
	if len(out.Decisions) != 4 {
		t.Errorf("the import recorded %d decisions, want one per node", len(out.Decisions))
	}

	named := platformImport(t, platformTestFormat(), platformCatalog(), `{"layout":"platform"}`, oneSource(platformSource))
	a, _ := json.Marshal(out)
	b, _ := json.Marshal(named)
	if !bytes.Equal(a, b) {
		t.Errorf("layout: platform drafts differently from no layout:\n  none: %s\nplatform: %s", a, b)
	}
}

// TestTheNodesLayoutDraftsEveryNodeAsAFormatWithNoPlatformDoes holds layout:
// nodes to the node-by-node draft: a format describing a platform, imported with
// layout nodes, returns byte for byte what the same format describing none
// returns with no settings — entities, decisions, diagnostics and files.
//
// 🔴 PLANTED DEFECT: in Import (import.go), drop `&& !settings.nodeLayout()` from
// the condition that hands a platform format to importPlatform, and this test
// fails: layout nodes is drafted as the platform.
func TestTheNodesLayoutDraftsEveryNodeAsAFormatWithNoPlatformDoes(t *testing.T) {
	t.Parallel()
	for _, source := range []string{platformSource, renderSource} {
		nodes := platformImport(t, platformTestFormat(), platformCatalog(), `{"layout":"nodes"}`, oneSource(source))
		plain := platformImport(t, testFormat(), platformCatalog(), "", oneSource(source))
		a, _ := json.Marshal(nodes)
		b, _ := json.Marshal(plain)
		if !bytes.Equal(a, b) {
			t.Errorf("layout nodes drafts differently from the node-by-node draft:\nnodes: %s\nplain: %s", a, b)
		}
	}
}

// TestAnUnknownLayoutIsDraftedAsThePlatformWithAWarning reads a layout setting
// naming neither layout: the source is drafted as the platform, and one warning
// names the value and the two layouts.
func TestAnUnknownLayoutIsDraftedAsThePlatformWithAWarning(t *testing.T) {
	t.Parallel()
	odd := platformImport(t, platformTestFormat(), platformCatalog(), `{"layout":"graph"}`, oneSource(platformSource))
	def := platformImport(t, platformTestFormat(), platformCatalog(), "", oneSource(platformSource))
	if !reflect.DeepEqual(odd.Files, def.Files) || !reflect.DeepEqual(odd.Decisions, def.Decisions) {
		t.Error("an unknown layout was not drafted as the platform")
	}
	want := []aggrega.Diagnostic{{Severity: "warn", Code: "unknown-layout",
		Message: `the layout setting "graph" is neither platform nor nodes, so the source is drafted as platform`}}
	if !reflect.DeepEqual(odd.Diagnostics, want) {
		t.Errorf("the import raised %+v, want %+v", odd.Diagnostics, want)
	}
}

// TestAPartNoSocketTakesStaysInTheFlowAndIsReported drives the three ways a part
// is placed in no socket — its Component satisfies no socket's selector, the
// platform describes no matcher, and a socket's selector does not read — and the
// part a socket after an unreadable one still takes. Each part placed nowhere
// stays in the flow, its decision says why, and a warning names it; an unreadable
// selector is reported once, however many parts tried it.
func TestAPartNoSocketTakesStaysInTheFlowAndIsReported(t *testing.T) {
	t.Parallel()
	const widget = `{"id":"flow-8","nodes":[{"id":"w","type":"generic","data":{"type":"Widget"}}],"edges":[]}` + "\n"
	const embedding = `{"mapping_overrides":{"testflow":{"Widget":{"component":"core/ai-model/openai-embedding","category":"ai-model"}}}}`

	t.Run("its Component satisfies no socket", func(t *testing.T) {
		out := platformImport(t, platformTestFormat(), platformCatalog(), embedding, oneSource(widget))
		reason := "it maps to core/ai-model/openai-embedding, which satisfies the selector of no socket core/ai-agent/testplat declares"
		d := decisionOf(t, out, "w")
		if d.Chosen != nil || d.Rationale != "kept in the flow core/ai-agent/testplat carries, with no Component of its own: "+reason {
			t.Errorf("the part was decided %+v: %q", d.Chosen, d.Rationale)
		}
		want := []aggrega.Diagnostic{{Severity: "warn", Code: "part-not-placed", SourceRef: "w",
			Message: "node w (Widget) is wired to no socket of core/ai-agent/testplat: " + reason + "; it stays in the flow"}}
		if !reflect.DeepEqual(out.Diagnostics, want) {
			t.Errorf("the import raised %+v, want %+v", out.Diagnostics, want)
		}
		if agg := out.Files[len(out.Files)-1].Content; strings.Contains(agg, "uses:") || strings.Contains(agg, "    w:\n") {
			t.Errorf("a part placed nowhere was drafted or wired:\n%s", agg)
		}
	})

	t.Run("the platform describes no matcher", func(t *testing.T) {
		format := platformTestFormat()
		format.Platform.Match = nil
		out := platformImport(t, format, platformCatalog(), "", oneSource(platformSource))
		for _, ref := range []string{"model-1", "finder"} {
			d := decisionOf(t, out, ref)
			if d.Chosen != nil || !strings.Contains(d.Rationale, "this importer reads no socket selector") {
				t.Errorf("%s was decided %+v: %q", ref, d.Chosen, d.Rationale)
			}
		}
		if len(out.Diagnostics) != 2 || out.Diagnostics[0].Code != "part-not-placed" || out.Diagnostics[1].Code != "part-not-placed" {
			t.Errorf("the import raised %+v; want one part-not-placed warning per part", out.Diagnostics)
		}
	})

	t.Run("a socket's selector does not read", func(t *testing.T) {
		catalog := platformCatalog(append([]aggrega.CatalogSocket{{Name: "broken", Selector: "=x"}}, testPlatformEntry.Sockets...)...)
		out := platformImport(t, platformTestFormat(), catalog, "", oneSource(platformSource))
		want := []aggrega.Diagnostic{{Severity: "warn", Code: "socket-selector-unreadable",
			Message: "the selector of the broken socket core/ai-agent/testplat declares does not read, so no part is wired to it: a term names no key"}}
		if !reflect.DeepEqual(out.Diagnostics, want) {
			t.Errorf("the import raised %+v, want %+v", out.Diagnostics, want)
		}
		if agg := out.Files[len(out.Files)-1].Content; !strings.Contains(agg, "        model:\n          model-1:\n") ||
			!strings.Contains(agg, "        tools:\n          finder:\n") {
			t.Errorf("the parts were not wired past the unreadable socket:\n%s", agg)
		}
	})
}

// TestACatalogHoldingNoPlatformDraftsItAndWiresNothing imports against a catalog
// view holding no entry for the platform: the platform is drafted carrying the
// flow all the same, no part is drafted or wired, each node that would have been
// a part stays in the flow with a decision and a warning naming the absent
// platform, and one warning names the absent platform.
func TestACatalogHoldingNoPlatformDraftsItAndWiresNothing(t *testing.T) {
	t.Parallel()
	out := platformImport(t, platformTestFormat(), standardCatalog, "", oneSource(platformSource))

	if len(out.Files) != 1 || out.Files[0].Path != "aggregate.yaml" {
		t.Fatalf("the import drafted %+v; want the Aggregate alone", out.Files)
	}
	agg := out.Files[0].Content
	for _, want := range []string{"    testplat:\n      type: core/ai-agent/testplat\n", "        flow_id: 'flow-7'\n",
		"        flow_definition: |\n"} {
		if !strings.Contains(agg, want) {
			t.Errorf("the Aggregate does not carry %q:\n%s", want, agg)
		}
	}
	if strings.Contains(agg, "uses:") || strings.Contains(agg, "the credential each connection") {
		t.Errorf("the Aggregate wires a part to a platform the catalog does not hold:\n%s", agg)
	}

	var codes []string
	for _, d := range out.Diagnostics {
		codes = append(codes, d.Code+" "+d.SourceRef)
	}
	if want := []string{"part-not-placed model-1", "part-not-placed finder", "platform-not-in-catalog "}; !reflect.DeepEqual(codes, want) {
		t.Errorf("the import raised %v, want %v", codes, want)
	}
	if want := "the workspace's catalog holds no core/ai-agent/testplat: the draft names it as the platform carrying the " +
		"flow and wires no part of the flow to it"; out.Diagnostics[2].Message != want {
		t.Errorf("the absent platform is reported as %q, want %q", out.Diagnostics[2].Message, want)
	}
	d := decisionOf(t, out, "model-1")
	if want := "kept in the flow core/ai-agent/testplat carries, with no Component of its own: it maps to " +
		"core/ai-model/openai, but the workspace's catalog holds no core/ai-agent/testplat, so no socket of it can be read"; d.Chosen != nil || d.Rationale != want {
		t.Errorf("the would-be part was decided %+v: %q, want no choice and %q", d.Chosen, d.Rationale, want)
	}
}

// plantedKey is a key-shaped value planted in a credential field.
const plantedKey = "sk-plantedKITkey0123456789abcdefXYZ"

// TestNoCredentialValueReachesTheDraft plants a key-shaped value in a field the
// test's format marks as a credential, and, apart, in an argument a server is
// started with, which the format writes empty too, and imports each under both
// layouts. The planted value is in the source — the search below finds it there,
// which is what says the search fires — and in nothing the import returns: no
// drafted file, no decision, no diagnostic. The carried definition is the source
// with the values the format locates written "" and every other byte kept, and
// one warning per value names the node and the field, and not the value.
//
// 🔴 PLANTED DEFECT: in carry (platform.go), replace
// `scrubbed, emptied, err := rewriteSource(src.Bytes, edits, creds)` with
// `scrubbed, emptied, err := src.Bytes, []Credential(nil), error(nil); _, _ = edits, creds`,
// and both platform rows fail: the planted value is in the drafted Aggregate.
func TestNoCredentialValueReachesTheDraft(t *testing.T) {
	t.Parallel()
	const tail = " in the flow the draft carries: no credential from the source reaches the draft"
	argument := func(n int) aggrega.Diagnostic {
		return aggrega.Diagnostic{Severity: "warn", Code: "credential-emptied", SourceRef: "finder",
			Message: "the whole value of args." + strconv.Itoa(n) + " of node finder is written empty in the flow the draft " +
				"carries, since " + testArgumentDetail + ": no credential from the source reaches the draft"}
	}
	for _, plant := range []struct {
		name, source string
		// aggregate is the Aggregate the source is drafted as, with what the
		// format locates written "".
		aggregate   string
		diagnostics []aggrega.Diagnostic
	}{
		{
			name:      "a credential field",
			source:    strings.Replace(platformSource, `"secretValues":{"api_key":""}`, `"secretValues":{"api_key":"`+plantedKey+`"}`, 1),
			aggregate: renderedPlatformAggregate(),
			diagnostics: []aggrega.Diagnostic{{Severity: "warn", Code: "credential-emptied", SourceRef: "model-1",
				Message: "the value of api_key, which node model-1 marks as a credential, is written empty" + tail}},
		},
		{
			name:        "an argument",
			source:      strings.Replace(platformSource, `{"type":"Finder"}`, `{"type":"Finder","args":["--verbose","--api-key=`+plantedKey+`"]}`, 1),
			aggregate:   strings.Replace(renderedPlatformAggregate(), `{"type":"Finder"}`, `{"type":"Finder","args":["",""]}`, 1),
			diagnostics: []aggrega.Diagnostic{argument(0), argument(1)},
		},
	} {
		holds := func(text string) bool { return strings.Contains(text, plantedKey) }
		if !holds(plant.source) {
			t.Fatalf("the needle: the search does not find the value planted in %s, so it finds nothing", plant.name)
		}
		for _, layout := range []string{"", `{"layout":"nodes"}`} {
			t.Run(plant.name+", layout "+layout, func(t *testing.T) {
				out := platformImport(t, platformTestFormat(), platformCatalog(), layout, oneSource(plant.source))
				whole, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("encode the output: %v", err)
				}
				if holds(string(whole)) {
					t.Errorf("the planted value reaches the import's output:\n%s", whole)
				}
				for _, f := range out.Files {
					if holds(f.Content) {
						t.Errorf("the planted value reaches %s:\n%s", f.Path, f.Content)
					}
				}
				if layout != "" {
					return
				}
				if agg := out.Files[len(out.Files)-1].Content; agg != plant.aggregate {
					t.Errorf("the Aggregate is not the source carried with the values the format locates emptied:\n%s\nwant\n%s",
						agg, plant.aggregate)
				}
				if !reflect.DeepEqual(out.Diagnostics, plant.diagnostics) {
					t.Errorf("the import raised %+v, want %+v", out.Diagnostics, plant.diagnostics)
				}
			})
		}
	}
}

// TestAFlowABlockCannotCarryIsNotCarried imports a source holding a line
// separator, U+2028, inside a string: a YAML reader takes it for a line break,
// which would end the literal block mid-text, so the platform is drafted with no
// definition and a comment saying so, one warning names the character, how often
// and where it first appears, and the rest of the draft is made as before.
func TestAFlowABlockCannotCarryIsNotCarried(t *testing.T) {
	t.Parallel()
	source := `{"id":"flow-9",
"nodes":[{"id":"model-1","type":"generic","data":{"type":"OpenAIModel","prompt":"one` + " " + `two"}}],
"edges":[]}
`
	out := platformImport(t, platformTestFormat(), platformCatalog(), "", oneSource(source))
	want := []aggrega.Diagnostic{{Severity: "warn", Code: "definition-not-carried",
		Message: "the flow is not carried in values.flow_definition: it holds a line-break character U+2028 (1 in all, " +
			"the first at line 2, column 85), which a YAML literal block cannot carry as written"}}
	if !reflect.DeepEqual(out.Diagnostics, want) {
		t.Errorf("the import raised %+v, want %+v", out.Diagnostics, want)
	}
	agg := out.Files[len(out.Files)-1].Content
	if strings.Contains(agg, "flow_definition: |") || !strings.Contains(agg,
		"        # flow_definition is left unset: the import's diagnostics say why the flow is not carried.\n") {
		t.Errorf("the Aggregate does not leave the definition unset with a comment:\n%s", agg)
	}
	if !strings.Contains(agg, "        flow_id: 'flow-9'\n") || !strings.Contains(agg, "          model-1:\n") {
		t.Errorf("the rest of the draft was not made:\n%s", agg)
	}
}

// TestTheFlowIdentifier reads the identifier a draft names its flow by: the
// source's own where it is written as a draft carries it, and otherwise the one
// derived from the carried bytes — pinned against Python's uuid module, as in
// TestDerivedFlowIDIsTheVersion5UUIDOfTheFormatAndTheBytes — with a comment
// saying it was derived and why.
func TestTheFlowIdentifier(t *testing.T) {
	t.Parallel()
	const tail = `"nodes":[{"id":"a","type":"generic","data":{"type":"ChatOutput"}}],"edges":[]}`
	for _, tc := range []struct {
		name, source, want string
	}{
		{name: "the source's own", source: `{"id":"flow-7",` + tail,
			want: "      values:\n        flow_id: 'flow-7'\n"},
		{name: "a source giving none", source: `{` + tail,
			want: "      values:\n        # flow_id is derived from the exported flow, which gives no identifier of its own.\n" +
				"        flow_id: '7a8479dd-34f4-51c1-bbab-f0e2b52fc58d'\n"},
		{name: "a source giving one a draft does not write as it is", source: `{"id":"has space",` + tail,
			want: "      values:\n        # flow_id is derived from the exported flow: its own identifier holds characters a " +
				"draft does not write as they are.\n        flow_id: '30d85388-0057-5e3a-8fd7-d3f89e86b81a'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := platformImport(t, platformTestFormat(), platformCatalog(), "", oneSource(tc.source))
			if agg := out.Files[len(out.Files)-1].Content; !strings.Contains(agg, tc.want) {
				t.Errorf("the Aggregate does not carry\n%s\nit reads\n%s", tc.want, agg)
			}
		})
	}
}

// TestSeveralSourcesDraftOnePlatformInstanceEach imports two sources of one flow
// and one that does not parse: each source that parses is drafted as its own
// instance of the platform, its parts keyed apart from the other's and its
// decisions referenced by its source's name; the one that does not parse halts
// with its name, and the others are drafted all the same.
func TestSeveralSourcesDraftOnePlatformInstanceEach(t *testing.T) {
	t.Parallel()
	out := platformImport(t, platformTestFormat(), platformCatalog(), "",
		aggrega.ImportSource{Name: "a.json", Bytes: []byte(platformSource)},
		aggrega.ImportSource{Name: "broken.json", Bytes: []byte(brokenSource)},
		aggrega.ImportSource{Name: "b.json", Bytes: []byte(platformSource)},
	)
	if len(out.Diagnostics) != 1 || out.Diagnostics[0].Code != "parse-failed" || out.Diagnostics[0].SourceRef != "broken.json" {
		t.Errorf("the import raised %+v; want the one source that does not parse halted by name", out.Diagnostics)
	}
	agg := out.Files[len(out.Files)-1].Content
	for _, want := range []string{
		"    testplat:\n      type: core/ai-agent/testplat\n",
		"    testplat-2:\n      type: core/ai-agent/testplat\n",
		"          model-1-2:\n            target: model-1-2\n",
		"          finder-2:\n            target: finder-2\n",
	} {
		if !strings.Contains(agg, want) {
			t.Errorf("the Aggregate does not carry\n%s\nit reads\n%s", want, agg)
		}
	}
	decisionOf(t, out, "a.json#model-1")
	decisionOf(t, out, "b.json#out")
}

// TestThePlatformLayoutIsDeterministic runs one platform import twice and
// requires byte-identical output.
func TestThePlatformLayoutIsDeterministic(t *testing.T) {
	t.Parallel()
	first, _ := json.Marshal(platformImport(t, platformTestFormat(), platformCatalog(), "", oneSource(platformSource)))
	second, _ := json.Marshal(platformImport(t, platformTestFormat(), platformCatalog(), "", oneSource(platformSource)))
	if !bytes.Equal(first, second) {
		t.Errorf("two platform imports of one source differ:\n first: %s\nsecond: %s", first, second)
	}
}
