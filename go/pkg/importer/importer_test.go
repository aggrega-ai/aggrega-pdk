// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// testFormatName names the format of the test's own the cases import.
const testFormatName = "testflow"

// testMappings is the mapping arm of the test's own format: one node type mapped
// with every signal it can carry, and one mapped to a component with nothing but
// the mapping and its category.
var testMappings = map[string]Mapping{
	"OpenAIModel": {
		Component: "core/ai-model/openai",
		Category:  "ai-model",
		Traits:    map[string]string{"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"},
		Vendor:    "openai",
	},
	"Finder": {Component: "core/ai-tool/retrieval", Category: "ai-tool"},
}

// testFormat is a format of the test's own. Its tree is an object holding a nodes
// array and an edges array, read through the package's own list readers; each node
// keeps its type, its value fields and its credential fields in its data, and a
// node whose outer type is note is not lifted.
func testFormat() Format {
	return Format{Name: testFormatName, Syntax: "json", Mappings: testMappings, Walk: walkTestTree}
}

func walkTestTree(tree json.RawMessage) ([]Node, []Edge) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(tree, &top); err != nil {
		return nil, nil
	}
	return NodeList(top["nodes"], liftTestNode), EdgeList(top["edges"])
}

func liftTestNode(id, outerType string, data map[string]json.RawMessage) (Node, bool) {
	if outerType == "note" {
		return Node{}, false
	}
	n := Node{ID: id}
	_ = json.Unmarshal(data["type"], &n.Type)
	_ = json.Unmarshal(data["fields"], &n.Fields)
	_ = json.Unmarshal(data["secrets"], &n.Secrets)
	return n, true
}

// parseRecorder is the parse callback the cases hand Import: it records the
// syntax it was asked for and hands each source back as its own tree, or fails a
// source whose bytes are exactly brokenSource.
type parseRecorder struct {
	syntaxes []string
}

const brokenSource = "not a tree"

func (p *parseRecorder) parse(syntax string, source []byte) (json.RawMessage, error) {
	p.syntaxes = append(p.syntaxes, syntax)
	if string(source) == brokenSource {
		return nil, errors.New("the source is not JSON")
	}
	return json.RawMessage(source), nil
}

// standardCatalog is the catalog view most cases import against: seven
// Components modelled on standard-library ai-model, ai-tool and ai-agent
// Components, written as the host writes a view — one entry per name, sorted by
// name, each entry's value key names sorted — with the labels a Component of
// each category carries.
var standardCatalog = &aggrega.CatalogProjection{Components: []aggrega.CatalogComponent{
	{
		Name: "core/ai-agent/flowise", Version: "1.0.0",
		Labels: map[string]string{"component.aggrega.ai/type": "server", "component.aggrega.ai/category": "ai-agent",
			"agent.aggrega.ai/autonomy": "copilot"},
		Values: []string{"chatflow_id", "override_config"},
	},
	{
		Name: "core/ai-agent/langflow", Version: "1.0.0",
		Labels: map[string]string{"component.aggrega.ai/type": "server", "component.aggrega.ai/category": "ai-agent",
			"agent.aggrega.ai/autonomy": "copilot"},
		Values: []string{"flow_id", "tweaks"},
	},
	{
		Name: "core/ai-model/anthropic", Version: "1.0.0",
		Labels: map[string]string{"component.aggrega.ai/type": "resource", "component.aggrega.ai/category": "ai-model",
			"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"},
		Values: []string{"model", "temperature"},
	},
	{
		Name: "core/ai-model/openai", Version: "1.0.0",
		Labels: map[string]string{"component.aggrega.ai/type": "resource", "component.aggrega.ai/category": "ai-model",
			"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"},
		Values: []string{"model", "strict_response_format", "temperature"},
	},
	{
		Name: "core/ai-model/openai-embedding", Version: "1.0.0",
		Labels: map[string]string{"component.aggrega.ai/type": "resource", "component.aggrega.ai/category": "ai-model",
			"model.aggrega.ai/capability": "embedding", "component.aggrega.ai/hosting": "managed"},
		Values: []string{"model"},
	},
	{
		Name: "core/ai-tool/http-request", Version: "1.0.0",
		Labels: map[string]string{"component.aggrega.ai/type": "function", "component.aggrega.ai/category": "ai-tool",
			"tool.aggrega.ai/impact": "mutating"},
		Values: []string{"method", "payload_schema_ref"},
	},
	{
		Name: "core/ai-tool/retrieval", Version: "1.0.0",
		Labels: map[string]string{"component.aggrega.ai/type": "function", "component.aggrega.ai/category": "ai-tool",
			"tool.aggrega.ai/impact": "read-only"},
		Values: []string{"side_effects", "top_k"},
	},
}}

// importTest runs Import over one source of the test's format against
// standardCatalog, with the settings document given ("" for none).
func importTest(t *testing.T, settings, source string) aggrega.ImportOutput {
	t.Helper()
	return importAgainst(t, standardCatalog, settings, source)
}

// importAgainst runs Import over one source of the test's format against the
// catalog view given (nil for none), with the settings document given ("" for
// none).
func importAgainst(t *testing.T, catalog *aggrega.CatalogProjection, settings, source string) aggrega.ImportOutput {
	t.Helper()
	in := aggrega.ImportInput{Format: testFormatName, Bytes: []byte(source), Catalog: catalog}
	if settings != "" {
		in.Settings = json.RawMessage(settings)
	}
	return Import(in, testFormat(), (&parseRecorder{}).parse)
}

// oneNode is a source holding one node of the type given, with the value and
// credential fields given.
func oneNode(typ string, fields, secrets []string) string {
	data := map[string]any{"type": typ}
	if fields != nil {
		data["fields"] = fields
	}
	if secrets != nil {
		data["secrets"] = secrets
	}
	raw, _ := json.Marshal(map[string]any{
		"nodes": []any{map[string]any{"id": "node-1", "type": "generic", "data": data}},
		"edges": []any{},
	})
	return string(raw)
}

// decisionFor returns the one decision an import of a one-node source records.
func decisionFor(t *testing.T, out aggrega.ImportOutput) aggrega.ImportDecision {
	t.Helper()
	if len(out.Decisions) != 1 {
		t.Fatalf("the import recorded %d decisions, want one: %+v", len(out.Decisions), out.Decisions)
	}
	return out.Decisions[0]
}

// TestTierCuts pins the four tiers at their cuts: a score at a cut is in the
// higher tier, and a score a hair below it in the lower.
func TestTierCuts(t *testing.T) {
	for _, tc := range []struct {
		score float64
		want  string
	}{
		{1, tierHigh},
		{0.85, tierHigh},
		{0.8499, tierMedium},
		{0.5, tierMedium},
		{0.4999, tierLow},
		{0.2, tierLow},
		{0.1999, tierNone},
		{0, tierNone},
	} {
		if got := tierFor(tc.score); got != tc.want {
			t.Errorf("tierFor(%v) = %s, want %s", tc.score, got, tc.want)
		}
	}
}

// TestTheFiveSignalsOfAMappedNode reads every candidate a mapped node is scored
// against, with each signal's contribution: the mapping names the top candidate
// (table), its category, traits, field overlap and vendor token each add their
// weight, and the other candidates draw only the signals they share.
func TestTheFiveSignalsOfAMappedNode(t *testing.T) {
	out := importTest(t, "", oneNode("OpenAIModel", []string{"model_name", "temperature"}, []string{"api_key"}))
	want := []aggrega.ImportCandidate{
		{ComponentName: "core/ai-model/openai", Score: 0.92, Tier: tierHigh,
			Signals: aggrega.CandidateSignals{Table: 0.5, Category: 0.2, DomainLabels: 0.15, Shape: 0.02, Name: 0.05}},
		{ComponentName: "core/ai-model/anthropic", Score: 0.3806, Tier: tierLow,
			Signals: aggrega.CandidateSignals{Category: 0.2, DomainLabels: 0.15, Shape: 0.025, Name: 0.0056}},
		{ComponentName: "core/ai-model/openai-embedding", Score: 0.325, Tier: tierLow,
			Signals: aggrega.CandidateSignals{Category: 0.2, DomainLabels: 0.075, Name: 0.05}},
		{ComponentName: "core/ai-tool/retrieval", Score: 0.0111, Tier: tierNone,
			Signals: aggrega.CandidateSignals{Name: 0.0111}},
	}
	if got := decisionFor(t, out).Candidates; !reflect.DeepEqual(got, want) {
		t.Errorf("the candidates are\n%+v\nwant\n%+v", got, want)
	}
}

// TestTiedCandidatesRankByName drives a node whose only signal is the category
// its type reads as: every candidate of that category ties, and the tie is broken
// by name.
func TestTiedCandidatesRankByName(t *testing.T) {
	out := importTest(t, "", oneNode("ChatOutput", nil, nil))
	var got []string
	for _, c := range decisionFor(t, out).Candidates {
		if c.Score != 0.2 || c.Tier != tierLow {
			t.Errorf("%s scored %v (%s); every tied candidate scores the category weight alone", c.ComponentName, c.Score, c.Tier)
		}
		got = append(got, c.ComponentName)
	}
	want := []string{"core/ai-model/anthropic", "core/ai-model/openai", "core/ai-model/openai-embedding"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the tied candidates are ranked %v, want %v", got, want)
	}
}

// TestTheCandidatesAreTheCatalogsComponents imports one node against a catalog
// view holding two Components no importer carries, and nothing else: the node is
// scored against exactly those two. A candidate's category is read from its
// category label, so the Component carrying none draws no category signal however
// its name reads, and its shape is weighed over the key names its values declare
// alone — the provides keys the first one declares play no part.
func TestTheCandidatesAreTheCatalogsComponents(t *testing.T) {
	catalog := &aggrega.CatalogProjection{Components: []aggrega.CatalogComponent{
		{
			Name: "acme/ai-model/internal-llm", Version: "1.2.0",
			Labels: map[string]string{"component.aggrega.ai/type": "server", "component.aggrega.ai/category": "ai-model",
				"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "self-hosted"},
			Values:   []string{"max_tokens", "model", "temperature"},
			Provides: []string{"endpoint", "model"},
		},
		{
			Name: "acme/ai-model/uncategorised", Version: "0.1.0",
			Labels: map[string]string{"component.aggrega.ai/type": "resource"},
			Values: []string{"model"},
		},
	}}
	d := decisionFor(t, importAgainst(t, catalog, "", oneNode("ChatModel", []string{"model", "temperature"}, nil)))

	want := []aggrega.ImportCandidate{
		{ComponentName: "acme/ai-model/internal-llm", Score: 0.2667, Tier: tierLow,
			Signals: aggrega.CandidateSignals{Category: 0.2, Shape: 0.0667}},
		{ComponentName: "acme/ai-model/uncategorised", Score: 0.05, Tier: tierNone,
			Signals: aggrega.CandidateSignals{Shape: 0.05}},
	}
	if !reflect.DeepEqual(d.Candidates, want) {
		t.Errorf("the candidates are\n%+v\nwant\n%+v", d.Candidates, want)
	}
	if want := (&aggrega.DecisionChoice{Target: "imported/ai-model/chatmodel", Mode: "draft", Tier: tierLow}); !reflect.DeepEqual(d.Chosen, want) {
		t.Errorf("the choice is %+v, want %+v", d.Chosen, want)
	}
	if want := "drafted a new component; the closest candidate acme/ai-model/internal-llm scored only 0.2667"; d.Rationale != want {
		t.Errorf("the rationale reads %q, want %q", d.Rationale, want)
	}
}

// TestACatalogHoldingNoComponentScoresNoCandidate imports, under auto-apply, a
// node that matches a standard Component on every signal: against a request
// carrying no catalog view, and against one whose view holds no Component, no
// candidate is scored, the node is drafted at the NONE tier with no hint, and
// nothing is referenced. Against standardCatalog the same node is referenced, so
// the two empty rows fail on the catalog alone.
func TestACatalogHoldingNoComponentScoresNoCandidate(t *testing.T) {
	const autoApply = `{"reuse_mode":"auto-apply-above-threshold"}`
	model := oneNode("OpenAIModel", []string{"model_name", "temperature"}, []string{"api_key"})

	if d := decisionFor(t, importAgainst(t, standardCatalog, autoApply, model)); d.Chosen == nil || d.Chosen.Mode != "reference" {
		t.Fatalf("against standardCatalog the node is decided %+v; the control wants it referenced", d.Chosen)
	}

	for _, tc := range []struct {
		name    string
		catalog *aggrega.CatalogProjection
	}{
		{name: "no catalog view", catalog: nil},
		{name: "a view holding no Component", catalog: &aggrega.CatalogProjection{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := importAgainst(t, tc.catalog, autoApply, model)
			d := decisionFor(t, out)
			if len(d.Candidates) != 0 {
				t.Errorf("the node was scored against %+v; the request carries no Component", d.Candidates)
			}
			if want := (&aggrega.DecisionChoice{Target: "imported/ai-model/openaimodel", Mode: "draft", Tier: tierNone}); !reflect.DeepEqual(d.Chosen, want) {
				t.Errorf("the choice is %+v, want %+v", d.Chosen, want)
			}
			if want := "drafted a new component; no candidate scored above the draft-new floor"; d.Rationale != want {
				t.Errorf("the rationale reads %q, want %q", d.Rationale, want)
			}
			if d.SwapHint != "" {
				t.Errorf("the hint reads %q; a node with no candidate carries none", d.SwapHint)
			}
			for _, e := range out.Entities {
				if e.Mode == "reference" {
					t.Errorf("%s was referenced; the request carries no Component to reference", e.Name)
				}
			}
		})
	}
}

// TestAtMostFourCandidatesAreReported drives a node sharing one field with every
// candidate, so every candidate scores above zero, and requires the four best —
// by score, then by name — and no more.
func TestAtMostFourCandidatesAreReported(t *testing.T) {
	out := importTest(t, "", oneNode("Widget", []string{"model", "top_k", "method", "flow_id", "chatflow_id"}, nil))
	var got []string
	for _, c := range decisionFor(t, out).Candidates {
		got = append(got, c.ComponentName)
	}
	want := []string{"core/ai-model/openai-embedding", "core/ai-agent/flowise", "core/ai-agent/langflow", "core/ai-model/anthropic"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the reported candidates are %v, want %v", got, want)
	}
}

// TestDecisionDispositions drives one node per disposition and reads the choice,
// its rationale and its hint, and the entity emitted for it: a HIGH match is
// referenced only under auto-apply at or above the threshold, and every other
// outcome drafts, with a hint whose strength tracks the tier.
func TestDecisionDispositions(t *testing.T) {
	const autoApply = `{"reuse_mode":"auto-apply-above-threshold"}`
	model := oneNode("OpenAIModel", []string{"model_name", "temperature"}, []string{"api_key"})
	for _, tc := range []struct {
		name, settings, source string
		mode, tier, target     string
		rationale, swapHint    string
	}{
		{
			name: "a HIGH match under hints-only is drafted with a strong hint", source: model,
			mode: "draft", tier: tierHigh, target: "imported/ai-model/openaimodel",
			rationale: "drafted a new component; core/ai-model/openai is a strong match at score 0.9200 but hints-only mode references nothing",
			swapHint:  "resembles core/ai-model/openai (score 0.9200); reference it by setting type: core/ai-model/openai",
		},
		{
			name: "a HIGH match under auto-apply at or above the threshold is referenced", settings: autoApply, source: model,
			mode: "reference", tier: tierHigh, target: "core/ai-model/openai",
			rationale: "referenced core/ai-model/openai at score 0.9200 (HIGH) under auto-apply",
			swapHint:  "swap to core/ai-model/anthropic (score 0.3806) by setting type: core/ai-model/anthropic",
		},
		{
			name:     "a HIGH match under auto-apply below the threshold is drafted",
			settings: `{"reuse_mode":"auto-apply-above-threshold","reuse_threshold":0.95}`, source: model,
			mode: "draft", tier: tierHigh, target: "imported/ai-model/openaimodel",
			rationale: "drafted a new component; core/ai-model/openai is a strong match at score 0.9200 but hints-only mode references nothing",
			swapHint:  "resembles core/ai-model/openai (score 0.9200); reference it by setting type: core/ai-model/openai",
		},
		{
			name: "a MEDIUM match is drafted, even under auto-apply", settings: autoApply, source: oneNode("Finder", nil, nil),
			mode: "draft", tier: tierMedium, target: "imported/ai-tool/finder",
			rationale: "drafted a new component; core/ai-tool/retrieval is a partial match at score 0.7000",
			swapHint:  "resembles core/ai-tool/retrieval (score 0.7000); consider referencing it",
		},
		{
			name: "a LOW match is drafted with no hint", source: oneNode("ChatOutput", nil, nil),
			mode: "draft", tier: tierLow, target: "imported/ai-model/chatoutput",
			rationale: "drafted a new component; the closest candidate core/ai-model/anthropic scored only 0.2000",
		},
		{
			name: "a node no candidate scores is drafted with no hint", source: oneNode("CustomPythonComponent", []string{"code"}, nil),
			mode: "draft", tier: tierNone, target: "imported/component/custompythoncomponent",
			rationale: "drafted a new component; no candidate scored above the draft-new floor",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := importTest(t, tc.settings, tc.source)
			d := decisionFor(t, out)
			want := &aggrega.DecisionChoice{Target: tc.target, Mode: tc.mode, Tier: tc.tier}
			if !reflect.DeepEqual(d.Chosen, want) {
				t.Errorf("the choice is %+v, want %+v", d.Chosen, want)
			}
			if d.Rationale != tc.rationale {
				t.Errorf("the rationale reads %q, want %q", d.Rationale, tc.rationale)
			}
			if d.SwapHint != tc.swapHint {
				t.Errorf("the hint reads %q, want %q", d.SwapHint, tc.swapHint)
			}
			if len(out.Entities) != 2 {
				t.Fatalf("the import emitted %d entities, want the node's and the Aggregate: %+v", len(out.Entities), out.Entities)
			}
			e := out.Entities[0]
			if e.Kind != "Component" || e.Mode != tc.mode || e.Name != tc.target ||
				e.SourceRef != "node-1" || e.DecisionRef != "decision:node-1" {
				t.Errorf("the emitted entity is %+v, want a %s of %s for node-1", e, tc.mode, tc.target)
			}
			if (e.Body == "") != (tc.mode == "reference") {
				t.Errorf("the %s entity carries the body %q; a reference carries none and a draft carries one", tc.mode, e.Body)
			}
		})
	}
}

// renderSource is a source of three nodes, a note, and edges: one repeated, one
// naming the note, one missing its target.
const renderSource = `{"nodes":[` +
	`{"id":"model-1","type":"generic","data":{"type":"OpenAIModel","fields":["model_name","temperature"],"secrets":["api_key"]}},` +
	`{"id":"Custom Logic","type":"generic","data":{"type":"CustomPythonComponent","fields":["code"]}},` +
	`{"id":"out","type":"generic","data":{"type":"ChatOutput"}},` +
	`{"id":"n1","type":"note","data":{"type":"Note"}}],` +
	`"edges":[` +
	`{"source":"model-1","target":"Custom Logic"},{"source":"model-1","target":"Custom Logic"},` +
	`{"source":"Custom Logic","target":"out"},{"source":"n1","target":"out"},{"source":"model-1"}]}`

// The files the human renderer writes for renderSource under the default settings.
const (
	renderedModel = `# resembles core/ai-model/openai (score 0.9200); reference it by setting type: core/ai-model/openai
apiVersion: aggrega.ai/v1beta1
kind: Component
metadata:
  name: imported/ai-model/openaimodel
  version: 0.1.0
  labels:
    component.aggrega.ai/type: resource
    component.aggrega.ai/category: ai-model
spec:
  featuresSchema:
    api_key:
      required: true
      inject: []
      description: Credential slot inferred from the source node; the recipe controls delivery.
  schema:
    values:
      type: object
      properties:
        model_name: { type: string }
        temperature: { type: string }
      additionalProperties: false
`
	renderedCustom = `apiVersion: aggrega.ai/v1beta1
kind: Component
metadata:
  name: imported/component/custompythoncomponent
  version: 0.1.0
  labels:
    component.aggrega.ai/type: resource
spec:
  schema:
    values:
      type: object
      properties:
        code: { type: string }
      additionalProperties: false
`
	renderedOutput = `apiVersion: aggrega.ai/v1beta1
kind: Component
metadata:
  name: imported/ai-model/chatoutput
  version: 0.1.0
  labels:
    component.aggrega.ai/type: resource
    component.aggrega.ai/category: ai-model
spec: {}
`
	renderedAggregate = `apiVersion: aggrega.ai/v1beta1
kind: Aggregate
metadata:
  name: imported/flow
  version: 0.1.0
  description: Draft Aggregate imported from a source in the testflow format.
spec:
  components:
    # resembles core/ai-model/openai (score 0.9200); reference it by setting type: core/ai-model/openai
    model-1:
      type: imported/ai-model/openaimodel
    custom-logic:
      type: imported/component/custompythoncomponent
      uses:
        model-1:
          model-1:
            target: model-1
    out:
      type: imported/ai-model/chatoutput
      uses:
        custom-logic:
          custom-logic:
            target: custom-logic
`
)

// TestANodeFedByTwoNodesGivesEachConnectionAnAliasOfItsOwn imports a source whose
// output node is fed by two nodes: the drafted Aggregate writes each of its two
// connections under the socket and the alias of the key of the node feeding it,
// so the two aliases differ — a component reads each connection by its alias,
// whatever socket it is wired into, and a build halts on two connections of one
// component under one alias. Two imports write the same Aggregate.
//
// 🔴 PLANTED DEFECT: in (*aggregateBuilder).entity (emit.go), write every
// connection under the alias link again: both connections share it, and the case
// fails.
func TestANodeFedByTwoNodesGivesEachConnectionAnAliasOfItsOwn(t *testing.T) {
	const source = `{"nodes":[` +
		`{"id":"model","type":"generic","data":{"type":"OpenAIModel"}},` +
		`{"id":"finder","type":"generic","data":{"type":"Finder"}},` +
		`{"id":"out","type":"generic","data":{"type":"ChatOutput"}}],` +
		`"edges":[{"source":"model","target":"out"},{"source":"finder","target":"out"}]}`
	aggregate := func(out aggrega.ImportOutput) string {
		for _, f := range out.Files {
			if f.Path == "aggregate.yaml" {
				return f.Content
			}
		}
		t.Fatalf("the import wrote no aggregate.yaml: %+v", out.Files)
		return ""
	}
	got := aggregate(importTest(t, "", source))
	want := "    out:\n      type: imported/ai-model/chatoutput\n      uses:\n" +
		"        finder:\n          finder:\n            target: finder\n" +
		"        model:\n          model:\n            target: model\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("the output node's connections are not\n%s\nin\n%s", want, got)
	}
	if again := aggregate(importTest(t, "", source)); again != got {
		t.Errorf("two imports of one source drafted different Aggregates:\n%s\nand\n%s", got, again)
	}
}

// TestDraftAndAggregateRendering pins, byte for byte, the files the human renderer
// writes: one per drafted Component in node order, its HIGH hint written above
// it, then the Aggregate keying each node by its identifier, each use once however
// many edges repeat it, and nothing for a note or for an edge naming one.
// emit_hints false drops every hint comment and nothing else; the llm renderer
// writes the decision record the human run recorded as its one file.
func TestDraftAndAggregateRendering(t *testing.T) {
	human := importTest(t, "", renderSource)
	want := []aggrega.ScaffoldFile{
		{Path: "components/imported-ai-model-openaimodel.yaml", Content: renderedModel},
		{Path: "components/imported-component-custompythoncomponent.yaml", Content: renderedCustom},
		{Path: "components/imported-ai-model-chatoutput.yaml", Content: renderedOutput},
		{Path: "aggregate.yaml", Content: renderedAggregate},
	}
	if !reflect.DeepEqual(human.Files, want) {
		t.Errorf("the human renderer wrote\n%+v\nwant\n%+v", human.Files, want)
	}
	if len(human.Diagnostics) != 0 {
		t.Errorf("the import raised %+v; this source reads with none", human.Diagnostics)
	}

	quiet := importTest(t, `{"emit_hints":false}`, renderSource)
	if len(quiet.Files) != len(want) {
		t.Fatalf("with emit_hints false the renderer wrote %d files, want %d", len(quiet.Files), len(want))
	}
	for i, f := range quiet.Files {
		var kept []string
		for _, line := range strings.SplitAfter(want[i].Content, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "# ") {
				kept = append(kept, line)
			}
		}
		if f.Path != want[i].Path || f.Content != strings.Join(kept, "") {
			t.Errorf("with emit_hints false, %s reads\n%s\nwant the same file without its hint comments", f.Path, f.Content)
		}
	}
	if !reflect.DeepEqual(quiet.Decisions, human.Decisions) {
		t.Error("emit_hints changed the decision record; it only governs the comments written into the drafts")
	}

	llm := importTest(t, `{"render_mode":"llm"}`, renderSource)
	record, err := json.Marshal(human.Decisions)
	if err != nil {
		t.Fatalf("encode the decision record: %v", err)
	}
	wantLLM := []aggrega.ScaffoldFile{{Path: "import-decisions.json", Content: string(record)}}
	if !reflect.DeepEqual(llm.Files, wantLLM) {
		t.Errorf("the llm renderer wrote %+v, want the decision record as its one file", llm.Files)
	}
}

// TestSettingsDefaults reads the settings an import runs with for an absent
// document, an empty one, one that is not JSON, and partial ones: every setting
// it does not set keeps its default.
func TestSettingsDefaults(t *testing.T) {
	type view struct {
		namespace, reuseMode, renderMode, layout, emptyCredentials string
		threshold                                                  float64
		hints, nodes                                               bool
	}
	defaults := view{namespace: "imported", reuseMode: "hints-only", renderMode: "human", layout: "platform", threshold: 0.85,
		hints: true, emptyCredentials: "none"}
	for _, tc := range []struct {
		name, doc string
		want      view
	}{
		{name: "absent", doc: "", want: defaults},
		{name: "empty object", doc: `{}`, want: defaults},
		{name: "not JSON", doc: `{"target_namespace":`, want: defaults},
		{name: "a namespace", doc: `{"target_namespace":"north"}`,
			want: view{namespace: "north", reuseMode: "hints-only", renderMode: "human", layout: "platform", threshold: 0.85,
				hints: true, emptyCredentials: "none"}},
		{name: "auto-apply at a threshold, no hints, llm",
			doc: `{"reuse_mode":"auto-apply-above-threshold","reuse_threshold":0.9,"emit_hints":false,"render_mode":"llm"}`,
			want: view{namespace: "imported", reuseMode: "auto-apply-above-threshold", renderMode: "llm", layout: "platform",
				threshold: 0.9, hints: false, emptyCredentials: "none"}},
		{name: "the platform layout, named", doc: `{"layout":"platform"}`, want: defaults},
		{name: "the nodes layout", doc: `{"layout":"nodes"}`,
			want: view{namespace: "imported", reuseMode: "hints-only", renderMode: "human", layout: "nodes", threshold: 0.85,
				hints: true, nodes: true, emptyCredentials: "none"}},
		{name: "a layout that is neither, kept for the warning", doc: `{"layout":"graph"}`,
			want: view{namespace: "imported", reuseMode: "hints-only", renderMode: "human", layout: "graph", threshold: 0.85,
				hints: true, emptyCredentials: "none"}},
		{name: "credentials emptied as marked", doc: `{"empty_credentials":"marked"}`,
			want: view{namespace: "imported", reuseMode: "hints-only", renderMode: "human", layout: "platform", threshold: 0.85,
				hints: true, emptyCredentials: "marked"}},
		{name: "every credential emptied", doc: `{"empty_credentials":"all","layout":"nodes"}`,
			want: view{namespace: "imported", reuseMode: "hints-only", renderMode: "human", layout: "nodes", threshold: 0.85,
				hints: true, nodes: true, emptyCredentials: "all"}},
		{name: "no credential emptied, named", doc: `{"empty_credentials":"none"}`, want: defaults},
		{name: "the credential setting read past a sibling that does not decode",
			doc: `{"mapping_overrides":7,"empty_credentials":"all"}`,
			want: view{namespace: "imported", reuseMode: "hints-only", renderMode: "human", layout: "platform", threshold: 0.85,
				hints: true, emptyCredentials: "all"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := parseSettings(json.RawMessage(tc.doc))
			removal, halt := readCredentialRemoval(json.RawMessage(tc.doc))
			if halt != nil {
				t.Fatalf("the credential setting halted: %+v", *halt)
			}
			got := view{namespace: s.TargetNamespace, reuseMode: s.ReuseMode, renderMode: s.RenderMode, layout: s.Layout,
				threshold: s.ReuseThreshold, hints: s.emitInlineHints(), nodes: s.nodeLayout(),
				emptyCredentials: removal.level}
			if got != tc.want {
				t.Errorf("the settings read %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestMappingOverridesTakePrecedence redirects a node type the arm maps through
// the mapping_overrides setting: the override's component draws the table signal
// in the arm's place. An override written for another format leaves the arm in
// force.
func TestMappingOverridesTakePrecedence(t *testing.T) {
	model := oneNode("OpenAIModel", []string{"model_name", "temperature"}, []string{"api_key"})
	for _, tc := range []struct {
		name, format, wantTop string
	}{
		{name: "an override for the format", format: testFormatName, wantTop: "core/ai-model/anthropic"},
		{name: "an override for another format", format: "otherflow", wantTop: "core/ai-model/openai"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := `{"mapping_overrides":{"` + tc.format + `":{"OpenAIModel":` +
				`{"component":"core/ai-model/anthropic","category":"ai-model","vendor":"anthropic"}}}}`
			candidates := decisionFor(t, importTest(t, settings, model)).Candidates
			if len(candidates) == 0 || candidates[0].ComponentName != tc.wantTop {
				t.Fatalf("the top candidate is %+v, want %s", candidates, tc.wantTop)
			}
			for _, c := range candidates {
				if (c.Signals.Table != 0) != (c.ComponentName == tc.wantTop) {
					t.Errorf("%s draws the table signal %v; only %s is the mapped component", c.ComponentName, c.Signals.Table, tc.wantTop)
				}
			}
		})
	}
}

// TestARequestForAnotherFormatYieldsNoNodes hands the importer a request for a
// format it does not serve: the source is parsed as the served syntax, yields no
// nodes, and is reported by the no-nodes warning, leaving an Aggregate with no
// components.
func TestARequestForAnotherFormatYieldsNoNodes(t *testing.T) {
	p := &parseRecorder{}
	out := Import(aggrega.ImportInput{Format: "otherflow", Bytes: []byte(renderSource)}, testFormat(), p.parse)
	if want := []string{"json"}; !reflect.DeepEqual(p.syntaxes, want) {
		t.Errorf("the parse callback was asked for %v, want %v", p.syntaxes, want)
	}
	wantDiag := []aggrega.Diagnostic{{Severity: "warn", Code: "no-nodes",
		Message: `source "" carried no recognisable otherflow nodes`}}
	if !reflect.DeepEqual(out.Diagnostics, wantDiag) {
		t.Errorf("the import raised %+v, want %+v", out.Diagnostics, wantDiag)
	}
	if len(out.Decisions) != 0 || len(out.Entities) != 1 || out.Entities[0].Kind != "Aggregate" {
		t.Errorf("the import recorded %+v and emitted %+v; want no decision and the Aggregate alone", out.Decisions, out.Entities)
	}
	if strings.Contains(out.Entities[0].Body, "type:") {
		t.Errorf("the Aggregate composes a component:\n%s", out.Entities[0].Body)
	}
}

// TestSeveralSources drives an import of several named sources: one that does
// not parse halts with its name while the others are imported; a node is
// referenced by its source's name, and two nodes of one identifier in two sources
// are keyed apart, each source's edges wiring its own nodes.
func TestSeveralSources(t *testing.T) {
	flow := `{"nodes":[` +
		`{"id":"model-1","type":"generic","data":{"type":"OpenAIModel"}},` +
		`{"id":"use","type":"generic","data":{"type":"CustomPythonComponent"}}],` +
		`"edges":[{"source":"model-1","target":"use"}]}`
	in := aggrega.ImportInput{Format: testFormatName, Sources: []aggrega.ImportSource{
		{Name: "a.json", Bytes: []byte(flow)},
		{Name: "broken.json", Bytes: []byte(brokenSource)},
		{Name: "b.json", Bytes: []byte(flow)},
	}}
	out := Import(in, testFormat(), (&parseRecorder{}).parse)

	wantDiag := []aggrega.Diagnostic{{Severity: "halt", Code: "parse-failed",
		Message:   `could not parse source "broken.json" as testflow: the source is not JSON`,
		SourceRef: "broken.json"}}
	if !reflect.DeepEqual(out.Diagnostics, wantDiag) {
		t.Errorf("the import raised %+v, want %+v", out.Diagnostics, wantDiag)
	}
	var refs []string
	for _, d := range out.Decisions {
		refs = append(refs, d.SourceNode.ID)
	}
	if want := []string{"a.json#model-1", "a.json#use", "b.json#model-1", "b.json#use"}; !reflect.DeepEqual(refs, want) {
		t.Errorf("the decisions reference %v, want %v", refs, want)
	}
	agg := out.Entities[len(out.Entities)-1].Body
	for _, want := range []string{
		"    use:\n      type: imported/component/custompythoncomponent\n      uses:\n        model-1:\n",
		"    use-2:\n      type: imported/component/custompythoncomponent\n      uses:\n        model-1-2:\n",
	} {
		if !strings.Contains(agg, want) {
			t.Errorf("the Aggregate does not carry\n%s\nit reads\n%s", want, agg)
		}
	}
}

// TestImportIsDeterministic runs one import twice and requires byte-identical
// output.
func TestImportIsDeterministic(t *testing.T) {
	first, err := json.Marshal(importTest(t, "", renderSource))
	if err != nil {
		t.Fatalf("encode the first output: %v", err)
	}
	second, err := json.Marshal(importTest(t, "", renderSource))
	if err != nil {
		t.Fatalf("encode the second output: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("two imports of one source differ:\n first: %s\nsecond: %s", first, second)
	}
}

// TestKebab pins the key and name-segment derivation: lowercase, every run of
// characters outside [a-z0-9] one hyphen, no hyphen at either end.
func TestKebab(t *testing.T) {
	for in, want := range map[string]string{
		"OpenAIModel":         "openaimodel",
		"chatOpenAI_0":        "chatopenai-0",
		"Custom Logic":        "custom-logic",
		"--Many   Separators": "many-separators",
		"LoopComponent-e7Ur0": "loopcomponent-e7ur0",
		"":                    "",
		"___":                 "",
	} {
		if got := kebab(in); got != want {
			t.Errorf("kebab(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestObjectEntries reads an object's members in the order the document writes
// them, nothing for a value that is not an object, and the members read before a
// document breaks off.
func TestObjectEntries(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      []string
	}{
		{name: "document order, not sorted", raw: `{"b":1,"a":{"x":[1]},"c":"s"}`, want: []string{"b", "a", "c"}},
		{name: "not an object", raw: `[{"a":1}]`},
		{name: "empty", raw: ``},
		{name: "broken off", raw: `{"a":1,"b":`, want: []string{"a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, e := range ObjectEntries(json.RawMessage(tc.raw)) {
				got = append(got, e.Key)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("the keys read are %v, want %v", got, tc.want)
			}
		})
	}
	entries := ObjectEntries(json.RawMessage(`{"a":{"x":[1]}}`))
	if len(entries) != 1 || string(entries[0].Value) != `{"x":[1]}` {
		t.Errorf("the member's value is not handed over undecoded: %+v", entries)
	}
}

// TestNodeListAndEdgeList reads the two list shapes: each node element's id, outer
// type and data handed to the lift, an element the lift declines left out, an
// absent member handed over empty; and each edge with both ends, an edge missing
// one or naming one by a non-string left out. A value that is not an array yields
// nothing.
func TestNodeListAndEdgeList(t *testing.T) {
	type seen struct{ id, outer, dataType string }
	var calls []seen
	nodes := NodeList(json.RawMessage(`[{"id":"a","type":"t1","data":{"type":"A"}},{"type":"note"},{"id":"c"}]`),
		func(id, outer string, data map[string]json.RawMessage) (Node, bool) {
			var typ string
			_ = json.Unmarshal(data["type"], &typ)
			calls = append(calls, seen{id, outer, typ})
			return Node{ID: id, Type: typ}, outer != "note"
		})
	if want := []seen{{"a", "t1", "A"}, {"", "note", ""}, {"c", "", ""}}; !reflect.DeepEqual(calls, want) {
		t.Errorf("the lift was handed %+v, want %+v", calls, want)
	}
	if want := []Node{{ID: "a", Type: "A"}, {ID: "c"}}; !reflect.DeepEqual(nodes, want) {
		t.Errorf("the lifted nodes are %+v, want %+v", nodes, want)
	}
	if got := NodeList(json.RawMessage(`{"id":"a"}`), func(string, string, map[string]json.RawMessage) (Node, bool) {
		t.Error("the lift was called for a value that is not an array")
		return Node{}, true
	}); got != nil {
		t.Errorf("a value that is not an array yields %+v, want nothing", got)
	}

	edges := EdgeList(json.RawMessage(`[{"source":"a","target":"b"},{"source":"a"},{"target":"b"},{"source":1,"target":"b"},{"source":"","target":"b"},{"source":"b","target":"c"}]`))
	if want := []Edge{{Source: "a", Target: "b"}, {Source: "b", Target: "c"}}; !reflect.DeepEqual(edges, want) {
		t.Errorf("the edges are %+v, want %+v", edges, want)
	}
	if got := EdgeList(json.RawMessage(`null`)); len(got) != 0 {
		t.Errorf("null yields the edges %+v, want none", got)
	}
}
