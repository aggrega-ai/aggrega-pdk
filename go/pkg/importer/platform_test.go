// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// testPlatformName is the platform Component of the test's own format.
const testPlatformName = "core/ai-agent/testplat"

// platformTestFormat is the test's own format, describing a platform of the
// test's own: its flow is carried in flow_definition and named by flow_id, the
// source's top-level id is its own identifier, every member of a node's
// data.secretValues is a value the format marks as a credential, every element
// of its data.args — the arguments a server is started with — a member of an
// MCP server's configuration, and every member of its data.headers an input the
// format marks as no credential that can carry one, and a socket's selector is
// read by testMatch.
func platformTestFormat() Format {
	f := testFormat()
	f.Platform = &Platform{
		Component:       testPlatformName,
		DefinitionValue: "flow_definition",
		IdentifierValue: "flow_id",
		Identifier:      testFlowID,
		Credentials:     testCredentials,
		Match:           testMatch,
		LeftForRelease: []LeftItem{{Path: "infra.image", Text: "the image running the platform"},
			{Text: "the admin slot, the administrator's password"}},
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

// testArgumentDetail and testHeaderDetail say why the test's format writes an
// argument and a header empty.
const (
	testArgumentDetail = "an argument a server is started with can carry a credential"
	testHeaderDetail   = "a header can carry a credential, though the format marks it as none"
)

func testCredentials(tree json.RawMessage) []Credential {
	var top struct {
		Nodes []struct {
			ID   string `json:"id"`
			Data struct {
				SecretValues json.RawMessage `json:"secretValues"`
				Args         json.RawMessage `json:"args"`
				Headers      json.RawMessage `json:"headers"`
			} `json:"data"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(tree, &top); err != nil {
		return nil
	}
	var out []Credential
	for i, n := range top.Nodes {
		for _, e := range ObjectEntries(n.Data.SecretValues) {
			out = append(out, Credential{Node: n.ID, Field: e.Key, Class: CredentialMarked,
				Pointer: Pointer("nodes", strconv.Itoa(i), "data", "secretValues", e.Key)})
		}
		var args []json.RawMessage
		_ = json.Unmarshal(n.Data.Args, &args)
		for j := range args {
			out = append(out, Credential{Node: n.ID, Field: "args." + strconv.Itoa(j), Class: CredentialServerMember,
				Pointer: Pointer("nodes", strconv.Itoa(i), "data", "args", strconv.Itoa(j)), Detail: testArgumentDetail})
		}
		for _, e := range ObjectEntries(n.Data.Headers) {
			out = append(out, Credential{Node: n.ID, Field: "headers." + e.Key, Class: CredentialUnmarked,
				Pointer: Pointer("nodes", strconv.Itoa(i), "data", "headers", e.Key), Detail: testHeaderDetail})
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
apiVersion: aggrega.ai/v1beta1
kind: Component
metadata:
  name: imported/ai-tool/finder
  version: 0.1.0
  labels:
    component.aggrega.ai/type: function
    component.aggrega.ai/category: ai-tool
spec: {}
`

// renderedPlatformRelease is the Release drafted for platformSource: an entry for
// the platform's instance, naming what the platform leaves for the Release and
// the credential it presents on each connection, and an empty entry for each part
// wired to it, in the Aggregate's order.
const renderedPlatformRelease = `apiVersion: aggrega.ai/v1beta1
kind: Release
metadata:
  name: imported/flow
  # environment is a drafted default, since no source names the environment this Release deploys to; change it to name the one it deploys to.
  environment: development
  version: 0.1.0
  description: Draft Release imported from a source in the testflow format.
spec:
  aggregate: imported/flow
  # manifest: left empty, since the import was given no target; an import given a target drafts the Manifest and names it here.
  manifest: ""
  components:
    # Left for the Release, since the source cannot supply it:
    #   - infra.image, the image running the platform
    #   - the admin slot, the administrator's password
    #   - uses.model.model-1.features.credential, the credential testplat presents to model-1 on its model connection
    #   - uses.tools.finder.features.credential, the credential testplat presents to finder on its tools connection
    testplat: {}
    model-1: {}
    finder: {}
`

// renderedPlatformAggregate is the Aggregate drafted for platformSource under the
// hints-only reuse mode and otherwise the default settings, its definition
// written with the double braces escaped by the test's own spelling of the
// escape.
func renderedPlatformAggregate() string {
	escaped := strings.Replace(platformSourceLine, "{{question}}", `{{"{{"}}question}}`, 1)
	return `apiVersion: aggrega.ai/v1beta1
kind: Aggregate
metadata:
  name: imported/flow
  version: 0.1.0
  description: Draft Aggregate imported from a source in the testflow format.
spec:
  components:
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
// chooses nothing, offers no swap and says why it is kept. The Release beside the
// Aggregate holds an entry for the instance and for each part. An attachment
// writing layout: platform drafts exactly the same. The import asks for the
// hints-only reuse mode, so the HIGH model match is drafted and its rendering read.
func TestThePlatformLayoutDraftsThePlatformCarryingTheFlow(t *testing.T) {
	t.Parallel()
	out := platformImport(t, platformTestFormat(), platformCatalog(), hintsOnly, oneSource(platformSource))

	want := []aggrega.ScaffoldFile{
		{Path: "components/imported-ai-model-openaimodel.yaml", Content: renderedModel},
		{Path: "components/imported-ai-tool-finder.yaml", Content: renderedFinder},
		{Path: "aggregate.yaml", Content: renderedPlatformAggregate()},
		{Path: ReleaseFileName, Content: renderedPlatformRelease},
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
		"Release draft imported/flow",
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

	named := platformImport(t, platformTestFormat(), platformCatalog(), `{"layout":"platform","reuse_mode":"hints-only"}`,
		oneSource(platformSource))
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
		if agg := aggregateFile(out); strings.Contains(agg, "uses:") || strings.Contains(agg, "    w:\n") {
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
		if agg := aggregateFile(out); !strings.Contains(agg, "        model:\n          model-1:\n") ||
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

	if len(out.Files) != 2 || out.Files[0].Path != "aggregate.yaml" || out.Files[1].Path != ReleaseFileName {
		t.Fatalf("the import drafted %+v; want the Aggregate and the Release alone", out.Files)
	}
	agg := out.Files[0].Content
	if release := out.Files[1].Content; !strings.HasSuffix(release, "  components:\n"+
		"    # Left for the Release, since the source cannot supply it:\n"+
		"    #   - infra.image, the image running the platform\n"+
		"    #   - the admin slot, the administrator's password\n"+
		"    testplat: {}\n") {
		t.Errorf("the Release does not name the platform's instance alone, leaving no connection's credential:\n%s",
			release)
	}
	for _, want := range []string{"    testplat:\n      type: core/ai-agent/testplat\n", "        flow_id: 'flow-7'\n",
		"        flow_definition: |\n"} {
		if !strings.Contains(agg, want) {
			t.Errorf("the Aggregate does not carry %q:\n%s", want, agg)
		}
	}
	if strings.Contains(agg, "uses:") {
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

// The key-shaped values a test plants, one per class of credential value the
// test's format locates: in a field it marks as a credential, in an argument a
// server is started with, and in a header it marks as no credential.
const (
	plantedMarked   = "sk-plantedKITmarked0123456789abcdef"
	plantedServer   = "sk-plantedKITserver0123456789abcdef"
	plantedUnmarked = "sk-plantedKITunmarked0123456789abcd"
)

// plantedFields writes, in place of model-1's empty api_key and the finder's
// bare data in platformSourceLine, an api_key of apiKey, two arguments and two
// headers, the second of which holds nothing.
func plantedFields(line, apiKey, arg0, arg1, auth string) string {
	line = strings.Replace(line, `"secretValues":{"api_key":""}`, `"secretValues":{"api_key":"`+apiKey+`"}`, 1)
	return strings.Replace(line, `{"type":"Finder"}`, `{"type":"Finder","args":["`+arg0+`","`+arg1+`"],`+
		`"headers":{"Authorization":"`+auth+`","Accept":""}}`, 1)
}

// carriedWarning and emptiedWarning are the reports of a value of the test's
// format carried as written and written empty.
func carriedWarning(node, field, why, level string) aggrega.Diagnostic {
	return aggrega.Diagnostic{Severity: "warn", Code: "credential-carried", SourceRef: node,
		Message: "the value of " + field + " of node " + node + " is carried as written in the flow the draft carries: " +
			why + ", and setting empty_credentials to " + level + " writes it empty"}
}

func emptiedWarning(node, field, detail string) aggrega.Diagnostic {
	if detail == "" {
		return aggrega.Diagnostic{Severity: "warn", Code: "credential-emptied", SourceRef: node,
			Message: "the value of " + field + ", which node " + node + " marks as a credential, is written empty in the " +
				"flow the draft carries"}
	}
	return aggrega.Diagnostic{Severity: "warn", Code: "credential-emptied", SourceRef: node,
		Message: "the whole value of " + field + " of node " + node + " is written empty in the flow the draft carries, " +
			"since " + detail}
}

// The reasons a carried value's report gives, by class.
const (
	markedWhy   = "its format marks the field as a credential"
	serverWhy   = "the field belongs to an MCP server's configuration, which can carry a key"
	unmarkedWhy = "its format marks the field as no credential, though it can carry one"
)

// TestTheEmptyCredentialsSettingDecidesWhatIsCarried plants a key-shaped value of
// each class the test's format locates — in a field it marks as a credential, in
// an argument a server is started with, and in a header it marks as none — and
// imports the flow under the empty_credentials setting absent and at each value
// it takes, each under the hints-only reuse mode, so the Aggregate read is the
// one renderedPlatformAggregate pins:
//
//   - absent and none write nothing empty: the flow is carried byte for byte,
//     every planted value in it as written and in the order written, and one
//     credential-carried warning per located value holding something — the
//     second argument and the header holding nothing raise none — naming the
//     node, the field and the setting value that writes it empty;
//   - marked writes empty the marked field and the server's arguments, each
//     reported under credential-emptied and none under credential-carried, and
//     carries the header as in the default;
//   - all writes every one of them empty.
//
// Under every value no planted value reaches a diagnostic, a decision or a
// comment of any drafted file — the search finds the values in the source,
// which is what says it fires — and under the node-by-node layout none reaches
// anything the import returns at all, since that draft reads no value.
//
// 🔴 PLANTED DEFECTS, each in credentials.go:
//
//   - in carryCredentials, replace `case removal.empties(c.Class):` with
//     `case true:`: every value is written empty under the default, and the
//     absent and none rows fail;
//   - in carryCredentials, replace
//     `case verdicts[i] == carried && holdsValue(tree, c):` with `case false:`:
//     the default raises no warning, and the absent and none rows fail;
//   - in readCredentialRemoval, replace `return credentialRemoval{level: level}, nil`
//     with `return credentialRemoval{level: emptyNone}, nil`: the setting reads
//     as none, and the marked and all rows fail.
func TestTheEmptyCredentialsSettingDecidesWhatIsCarried(t *testing.T) {
	t.Parallel()
	source := plantedFields(platformSourceLine, plantedMarked, "--verbose", "--api-key="+plantedServer,
		"Bearer "+plantedUnmarked) + "\n"
	planted := []string{plantedMarked, plantedServer, plantedUnmarked}
	for _, needle := range planted {
		if !strings.Contains(source, needle) {
			t.Fatalf("the needle: the search does not find %s in the source, so it finds nothing", needle)
		}
	}
	carriedAs := func(apiKey, arg0, arg1, auth string) string {
		agg := renderedPlatformAggregate()
		agg = strings.Replace(agg, `"secretValues":{"api_key":""}`, `"secretValues":{"api_key":"`+apiKey+`"}`, 1)
		return strings.Replace(agg, `{"type":"Finder"}`, `{"type":"Finder","args":["`+arg0+`","`+arg1+`"],`+
			`"headers":{"Authorization":"`+auth+`","Accept":""}}`, 1)
	}
	asWritten := carriedAs(plantedMarked, "--verbose", "--api-key="+plantedServer, "Bearer "+plantedUnmarked)
	defaultWarnings := []aggrega.Diagnostic{
		carriedWarning("model-1", "api_key", markedWhy, "marked or all"),
		carriedWarning("finder", "args.0", serverWhy, "marked or all"),
		carriedWarning("finder", "args.1", serverWhy, "marked or all"),
		carriedWarning("finder", "headers.Authorization", unmarkedWhy, "all"),
	}
	for _, tc := range []struct {
		name, settings, aggregate string
		diagnostics               []aggrega.Diagnostic
	}{
		{name: "absent", settings: hintsOnly, aggregate: asWritten, diagnostics: defaultWarnings},
		{name: "none", settings: `{"empty_credentials":"none","reuse_mode":"hints-only"}`, aggregate: asWritten,
			diagnostics: defaultWarnings},
		{name: "marked", settings: `{"empty_credentials":"marked","reuse_mode":"hints-only"}`,
			aggregate: carriedAs("", "", "", "Bearer "+plantedUnmarked),
			diagnostics: []aggrega.Diagnostic{
				emptiedWarning("model-1", "api_key", ""),
				emptiedWarning("finder", "args.0", testArgumentDetail),
				emptiedWarning("finder", "args.1", testArgumentDetail),
				carriedWarning("finder", "headers.Authorization", unmarkedWhy, "all"),
			}},
		{name: "all", settings: `{"empty_credentials":"all","reuse_mode":"hints-only"}`, aggregate: carriedAs("", "", "", ""),
			diagnostics: []aggrega.Diagnostic{
				emptiedWarning("model-1", "api_key", ""),
				emptiedWarning("finder", "args.0", testArgumentDetail),
				emptiedWarning("finder", "args.1", testArgumentDetail),
				emptiedWarning("finder", "headers.Authorization", testHeaderDetail),
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := platformImport(t, platformTestFormat(), platformCatalog(), tc.settings, oneSource(source))
			if agg := aggregateFile(out); agg != tc.aggregate {
				t.Errorf("the Aggregate is not the source carried as the setting asks:\n%s\nwant\n%s", agg, tc.aggregate)
			}
			if !reflect.DeepEqual(out.Diagnostics, tc.diagnostics) {
				t.Errorf("the import raised %+v, want %+v", out.Diagnostics, tc.diagnostics)
			}
			noPlantedValueOutsideTheFlow(t, out, planted)
		})
		t.Run(tc.name+", node by node", func(t *testing.T) {
			settings := `{"layout":"nodes"}`
			if tc.settings != "" {
				settings = strings.Replace(tc.settings, "{", `{"layout":"nodes",`, 1)
			}
			out := platformImport(t, platformTestFormat(), platformCatalog(), settings, oneSource(source))
			whole, err := json.Marshal(out)
			if err != nil {
				t.Fatalf("encode the output: %v", err)
			}
			for _, needle := range planted {
				if strings.Contains(string(whole), needle) {
					t.Errorf("the node-by-node draft carries %s:\n%s", needle, whole)
				}
			}
			for _, d := range out.Diagnostics {
				if strings.HasPrefix(d.Code, "credential-") {
					t.Errorf("the node-by-node draft reports a credential value: %+v", d)
				}
			}
		})
	}
}

// noPlantedValueOutsideTheFlow fails the test where a planted value reaches a
// diagnostic or a decision of out, a drafted file other than the Aggregate — the
// Release among them — or a line of the Aggregate outside the flow it carries.
func noPlantedValueOutsideTheFlow(t *testing.T, out aggrega.ImportOutput, planted []string) {
	t.Helper()
	reported, err := json.Marshal(struct {
		D []aggrega.Diagnostic
		R []aggrega.ImportDecision
	}{out.Diagnostics, out.Decisions})
	if err != nil {
		t.Fatalf("encode the diagnostics and decisions: %v", err)
	}
	for _, needle := range planted {
		if strings.Contains(string(reported), needle) {
			t.Errorf("%s reaches a diagnostic or a decision:\n%s", needle, reported)
		}
		for _, f := range out.Files {
			if f.Path != "aggregate.yaml" {
				if strings.Contains(f.Content, needle) {
					t.Errorf("%s reaches %s:\n%s", needle, f.Path, f.Content)
				}
				continue
			}
			for _, line := range strings.Split(f.Content, "\n") {
				if strings.Contains(line, needle) && !strings.HasPrefix(line, "          {") {
					t.Errorf("%s reaches a line of the Aggregate outside the flow it carries: %q", needle, line)
				}
			}
		}
	}
}

// TestASettingValueTheImportDoesNotTakeHaltsIt imports the planted flow with the
// empty_credentials setting at values it does not take — another text, the
// empty text, a boolean, null and an object — and the import drafts nothing:
// no entity, no file, no decision, and one halting diagnostic naming the setting
// and the value as written. The node-by-node draft reads no value and so not the
// setting, and is drafted as it is without it.
//
// 🔴 PLANTED DEFECT: in readCredentialRemoval (credentials.go), replace
// `return credentialRemoval{}, &aggrega.Diagnostic{` with
// `return credentialRemoval{level: emptyNone}, nil; _ = &aggrega.Diagnostic{`
// (and drop the now unreachable rest of the statement): every value reads as
// none, the flow is carried, and every row fails.
func TestASettingValueTheImportDoesNotTakeHaltsIt(t *testing.T) {
	t.Parallel()
	source := plantedFields(platformSourceLine, plantedMarked, "--verbose", "--api-key="+plantedServer,
		"Bearer "+plantedUnmarked) + "\n"
	for _, value := range []string{`"mark"`, `""`, `true`, `null`, `{"level":"all"}`} {
		t.Run(value, func(t *testing.T) {
			out := platformImport(t, platformTestFormat(), platformCatalog(), `{"empty_credentials":`+value+`}`,
				oneSource(source))
			want := []aggrega.Diagnostic{{Severity: "halt", Code: "unknown-empty-credentials",
				Message: "the empty_credentials setting is " + value + ", which is none of none, marked and all, so the " +
					"import drafts nothing: an import writes a credential value empty only where the setting asks for it"}}
			if !reflect.DeepEqual(out.Diagnostics, want) {
				t.Errorf("the import raised %+v, want %+v", out.Diagnostics, want)
			}
			if len(out.Files) != 0 || len(out.Entities) != 0 || len(out.Decisions) != 0 {
				t.Errorf("the import drafted %d file(s), %d entit(ies) and %d decision(s), want none", len(out.Files),
					len(out.Entities), len(out.Decisions))
			}
			nodes := platformImport(t, platformTestFormat(), platformCatalog(),
				`{"layout":"nodes","empty_credentials":`+value+`}`, oneSource(source))
			if len(nodes.Files) == 0 {
				t.Errorf("the node-by-node draft drafted nothing: %+v", nodes.Diagnostics)
			}
			for _, d := range nodes.Diagnostics {
				if d.Severity == "halt" {
					t.Errorf("the node-by-node draft halted: %+v", d)
				}
			}
		})
	}
}

// TestADraftingImporterDecidesAsThePlatformDraftDoes hands CarryCredentials — the
// decision an importer drafting values of its own calls — a drafted JSON
// document holding a value of each class, at each value of the setting, and at
// one it does not take. Under each it writes and reports exactly what the
// platform draft writes and reports for the same values: the same values carried
// or emptied, under the same codes, in the same words but for where the
// document is written.
func TestADraftingImporterDecidesAsThePlatformDraftDoes(t *testing.T) {
	t.Parallel()
	doc := plantedFields(platformSourceLine, plantedMarked, "--verbose", "--api-key="+plantedServer,
		"Bearer "+plantedUnmarked)
	located := testCredentials(json.RawMessage(doc))
	for _, settings := range []string{"", `{"empty_credentials":"none"}`, `{"empty_credentials":"marked"}`,
		`{"empty_credentials":"all"}`} {
		t.Run("settings "+settings, func(t *testing.T) {
			written, diags, err := CarryCredentials(json.RawMessage(settings), []byte(doc), "in the Release the draft writes",
				located)
			if err != nil {
				t.Fatalf("the decision refused the document: %v", err)
			}
			out := platformImport(t, platformTestFormat(), platformCatalog(), settings, oneSource(doc+"\n"))
			agg := aggregateFile(out)
			if !strings.Contains(agg, "          "+strings.Replace(string(written), "{{question}}", `{{"{{"}}question}}`, 1)+
				"\n") {
				t.Errorf("the drafted document reads\n%s\nwhich is not the flow the platform draft carries:\n%s", written, agg)
			}
			var want []aggrega.Diagnostic
			for _, d := range out.Diagnostics {
				d.Message = strings.Replace(d.Message, "in the flow the draft carries", "in the Release the draft writes", 1)
				want = append(want, d)
			}
			if !reflect.DeepEqual(diags, want) {
				t.Errorf("the decision raised %+v, want %+v", diags, want)
			}
		})
	}
	written, diags, err := CarryCredentials(json.RawMessage(`{"empty_credentials":"some"}`), []byte(doc), "here", located)
	if err != nil || written != nil || len(diags) != 1 || diags[0].Severity != "halt" ||
		diags[0].Code != "unknown-empty-credentials" {
		t.Errorf("a value the setting does not take wrote %q and raised %+v (%v), want nothing and one halt", written,
			diags, err)
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
	agg := aggregateFile(out)
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
			if agg := aggregateFile(out); !strings.Contains(agg, tc.want) {
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
	agg := aggregateFile(out)
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
