// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"text/template"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// testModuleName is the Aggregate the test's module format drafts an entry of.
const testModuleName = "core/testmod"

// moduleSourceLine is a flow of two nodes: a mapped model holding a key-shaped
// value in the field the test's format marks as a credential, and a chat output
// whose prompt writes a variable in double braces.
const moduleSourceLine = `{"id":"flow-7","nodes":[` +
	`{"id":"model-1","type":"generic","data":{"type":"OpenAIModel","secretValues":{"api_key":"` + plantedMarked + `"}}},` +
	`{"id":"out","type":"generic","data":{"type":"ChatOutput","prompt":"Answer {{question}}"}}],"edges":[]}`

const moduleSource = moduleSourceLine + "\n"

// emptyRecorder is the EmptyInSource of the test's module format: it records
// the values it is asked to write empty and writes them as the format's own
// writer would, through Rewrite. Where also is set it writes that value empty
// too — a writer changing more than it was asked to — and where fail is set it
// refuses.
type emptyRecorder struct {
	mu    sync.Mutex
	asked [][]string
	also  string
	fail  bool
}

func (r *emptyRecorder) empty(source []byte, credentials []Credential) ([]byte, error) {
	r.mu.Lock()
	var pointers []string
	for _, c := range credentials {
		pointers = append(pointers, c.Pointer)
	}
	r.asked = append(r.asked, pointers)
	r.mu.Unlock()
	if r.fail {
		return nil, errors.New("the layout is not one the format reads")
	}
	var edits []Edit
	for _, p := range pointers {
		edits = append(edits, Edit{Pointer: p, Value: json.RawMessage(`""`)})
	}
	if r.also != "" {
		edits = append(edits, Edit{Pointer: r.also, Value: json.RawMessage(`""`)})
	}
	return Rewrite(source, edits)
}

// moduleTestFormat is the test's format with a platform running as the module
// testModuleName, which takes the flow in its flow parameter; the platform says
// a model node names a model it configures for itself; empty writes credential
// values.
func moduleTestFormat(empty *emptyRecorder) Format {
	f := testFormat()
	f.Platform = &Platform{
		Component:       testPlatformName,
		DefinitionValue: "flow_definition",
		IdentifierValue: "flow_id",
		Identifier:      testFlowID,
		Credentials:     testCredentials,
		Match:           testMatch,
		Module:          &Module{Aggregate: testModuleName, Parameter: "flow"},
		Configured: func(_ json.RawMessage, node Node) string {
			if node.Type == "OpenAIModel" {
				return "the model of the provider openai"
			}
			return ""
		},
		EmptyInSource:  empty.empty,
		LeftForRelease: []string{"infra.owner, the platform's owner"},
	}
	return f
}

// renderedModuleAggregate is the Aggregate the module form drafts for a flow
// carried as text, written with the double braces escaped by the test's own
// spelling of the escape.
func renderedModuleAggregate(flow string) string {
	escaped := strings.Replace(flow, "{{question}}", `{{"{{"}}question}}`, 1)
	return `apiVersion: aggrega.ai/v1alpha1
kind: Aggregate
metadata:
  name: imported/flow
  version: 0.1.0
  description: Draft Aggregate imported from a source in the testflow format.
spec:
  aggregates:
    # An import sees the Components of the workspace's catalog and none of its Aggregates, so it does not check that the catalog holds core/testmod.
    # Left for the Release, since the source cannot supply it:
    #   - infra.owner, the platform's owner
    testmod:
      type: core/testmod
      parameters:
        flow: |
          ` + escaped + `
`
}

// keptByModule is the rationale of a node the module form keeps in the flow,
// opening with what it names where it names something.
func keptByModule(names string) string {
	r := "kept in the flow core/ai-agent/testplat carries, with no Component of its own: "
	if names != "" {
		r += "it names " + names + ", and core/ai-agent/testplat configures that for itself; "
	}
	return r + "the draft composes core/testmod, which wires every connection of core/ai-agent/testplat itself, so " +
		"no node of the flow is wired"
}

// TestTheModuleFormDraftsAnEntryOfTheModuleCarryingTheFlow pins, byte for byte,
// the module form of the platform draft under the default settings: the drafted
// Aggregate composes one entry of the module, keyed by the last segment of its
// name, carrying the flow as written — its double braces escaped — in the
// module's parameter, with no identifier and nothing wired, and the comment
// naming what the Release supplies and that the import does not check the
// catalog for the module. No Component is drafted: the entities are the module,
// referenced, and the drafted Aggregate. Every node is kept in the flow; the node
// the format says names what the platform configures for itself says so in its
// decision and in a part-configured-in-platform warning, after the report of the
// credential value carried as written. The format's writer is never asked.
//
// 🔴 PLANTED DEFECT: in importModule (module.go), drop the configuredInPlatform
// warning: the case fails on the diagnostics.
func TestTheModuleFormDraftsAnEntryOfTheModuleCarryingTheFlow(t *testing.T) {
	t.Parallel()
	empty := &emptyRecorder{}
	out := platformImport(t, moduleTestFormat(empty), platformCatalog(), "", oneSource(moduleSource))

	want := []aggrega.ScaffoldFile{{Path: "aggregate.yaml", Content: renderedModuleAggregate(moduleSourceLine)}}
	if !reflect.DeepEqual(out.Files, want) {
		t.Errorf("the module form wrote\n%+v\nwant\n%+v", out.Files, want)
	}
	var kinds []string
	for _, e := range out.Entities {
		kinds = append(kinds, e.Kind+" "+e.Mode+" "+e.Name)
	}
	if want := []string{"Aggregate reference " + testModuleName, "Aggregate draft imported/flow"}; !reflect.DeepEqual(kinds, want) {
		t.Errorf("the import emitted %v, want %v", kinds, want)
	}
	wantDiags := []aggrega.Diagnostic{
		carriedWarning("model-1", "api_key", markedWhy, "marked or all"),
		{Severity: "warn", Code: "part-configured-in-platform", SourceRef: "model-1",
			Message: "node model-1 (OpenAIModel) names the model of the provider openai, and core/ai-agent/testplat " +
				"configures that for itself rather than reaching it through a socket, so the draft wires no part for it"},
	}
	if !reflect.DeepEqual(out.Diagnostics, wantDiags) {
		t.Errorf("the import raised\n%+v\nwant\n%+v", out.Diagnostics, wantDiags)
	}
	for ref, rationale := range map[string]string{
		"model-1": keptByModule("the model of the provider openai"),
		"out":     keptByModule(""),
	} {
		d := decisionOf(t, out, ref)
		if d.Chosen != nil || d.Rationale != rationale {
			t.Errorf("node %s was decided %+v: %q, want no choice and %q", ref, d.Chosen, d.Rationale, rationale)
		}
	}
	if len(empty.asked) != 0 {
		t.Errorf("the format's writer was asked %v under the default; a value carried as written is never rewritten",
			empty.asked)
	}
}

// TestTheModuleFormWritesValuesEmptyThroughTheFormatAndCarriesOnlyAMatchingTree
// imports the flow whose model node holds a key-shaped value under
// empty_credentials marked, with the format's own writer:
//
//   - a writer writing exactly the value it is asked to: it is asked about that
//     value alone, the flow is carried with the value written empty and every
//     other byte as written, and the value is reported under credential-emptied;
//   - a writer also writing another value empty, so the host's parser reads the
//     result as a tree differing beyond the value asked about: the flow is not
//     carried, the parameter is left unset, and definition-not-carried says why;
//   - a writer refusing: the flow is not carried, and the refusal is reported.
//
// No planted value reaches a diagnostic or a drafted file where the flow is not
// carried, nor the draft where it is.
//
// 🔴 PLANTED DEFECT: in sourceWriter.write (credentials.go), replace
// `if !sameTree(w.reparse, out, want) {` with `if false {`: the second row
// carries the flow, and fails.
func TestTheModuleFormWritesValuesEmptyThroughTheFormatAndCarriesOnlyAMatchingTree(t *testing.T) {
	t.Parallel()
	if !strings.Contains(moduleSource, plantedMarked) {
		t.Fatalf("the needle: the search does not find %s in the source, so it finds nothing", plantedMarked)
	}
	apiKey := "/nodes/0/data/secretValues/api_key"
	notCarried := "the flow is not carried in parameters.flow: its bytes with its credential values written empty do " +
		"not read back as its tree with exactly those values emptied, so a credential it holds could not be kept out of " +
		"the draft"
	for _, tc := range []struct {
		name    string
		writer  *emptyRecorder
		carried bool
		message string
	}{
		{name: "exactly the value asked", writer: &emptyRecorder{}, carried: true,
			message: "the value of api_key, which node model-1 marks as a credential, is written empty in the flow the " +
				"draft carries"},
		{name: "a writer changing another value", writer: &emptyRecorder{also: "/id"}, message: notCarried},
		{name: "a writer refusing", writer: &emptyRecorder{fail: true},
			message: "the flow is not carried in parameters.flow: its credential values could not be written empty in " +
				"its own bytes: the layout is not one the format reads, so a credential it holds could not be kept out " +
				"of the draft"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := platformImport(t, moduleTestFormat(tc.writer), platformCatalog(), `{"empty_credentials":"marked"}`,
				oneSource(moduleSource))
			if !reflect.DeepEqual(tc.writer.asked, [][]string{{apiKey}}) {
				t.Errorf("the writer was asked %v, want the one value holding something", tc.writer.asked)
			}
			if len(out.Diagnostics) < 1 || out.Diagnostics[0].Message != tc.message {
				t.Fatalf("the import raised %+v, want first %q", out.Diagnostics, tc.message)
			}
			agg := out.Files[0].Content
			emptied := strings.Replace(moduleSourceLine, plantedMarked, "", 1)
			if tc.carried {
				if want := renderedModuleAggregate(emptied); agg != want {
					t.Errorf("the draft is\n%s\nwant\n%s", agg, want)
				}
			} else if !strings.Contains(agg, "      # parameters.flow is left unset: the import's diagnostics say why the "+
				"flow is not carried.\n") || strings.Contains(agg, "parameters:") {
				t.Errorf("the draft carries a flow its writer did not write as asked:\n%s", agg)
			}
			for _, f := range out.Files {
				if strings.Contains(f.Content, plantedMarked) {
					t.Errorf("%s carries the planted value", f.Path)
				}
			}
			for _, d := range out.Diagnostics {
				if strings.Contains(d.Message, plantedMarked) {
					t.Errorf("a diagnostic carries the planted value: %q", d.Message)
				}
			}
		})
	}
}

// passedDownDescription is the description the test's passed-down parameter is
// declared with: double quotes, a backslash and two opening braces, each of which
// a double-quoted scalar of an Aggregate evaluated as a template must carry as
// written.
const passedDownDescription = `The key the "platform" signs with, {{ not an action }} and a back\slash.`

// passingDownFormat is moduleTestFormat with the module's signing_key parameter
// passed down.
func passingDownFormat(empty *emptyRecorder) Format {
	f := moduleTestFormat(empty)
	f.Platform.Module.PassedDown = []PassedDownParameter{
		{Name: "signing_key", Description: passedDownDescription},
	}
	return f
}

// passedDownHead is the head of the Aggregate the module form drafts with
// signing_key passed down, up to the first entry's key: the parameter declared
// once, required with no default, and the comment above each entry naming it
// first among what the Release supplies.
const passedDownHead = "apiVersion: aggrega.ai/v1alpha1\n" +
	"kind: Aggregate\n" +
	"metadata:\n" +
	"  name: imported/flow\n" +
	"  version: 0.1.0\n" +
	"  description: Draft Aggregate imported from a source in the testflow format.\n" +
	"spec:\n" +
	"  parameters:\n" +
	"    signing_key:\n" +
	"      type: string\n" +
	"      required: true\n" +
	"      minLength: 1\n" +
	"      description: \"The key the \\\"platform\\\" signs with, {{`{{`}} not an action }} and a back\\\\slash.\"\n" +
	"  aggregates:\n"

// passedDownComment is the comment the module form writes above each entry with
// signing_key passed down.
const passedDownComment = "    # An import sees the Components of the workspace's catalog and none of its Aggregates, so it does " +
	"not check that the catalog holds core/testmod.\n" +
	"    # Left for the Release, since the source cannot supply it:\n" +
	"    #   - the signing_key parameter declared above, which this entry passes down to core/testmod\n" +
	"    #   - infra.owner, the platform's owner\n"

// evaluated evaluates a drafted Aggregate as a template, as the compiler does
// before it reads the Aggregate as YAML, with the Release fulfilling signing_key
// as fulfilled.
func evaluated(t *testing.T, aggregate, fulfilled string) string {
	t.Helper()
	tmpl, err := template.New("aggregate").Parse(aggregate)
	if err != nil {
		t.Fatalf("the drafted Aggregate does not parse as a template: %v\n%s", err, aggregate)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, map[string]any{"parameters": map[string]any{"signing_key": fulfilled}}); err != nil {
		t.Fatalf("the drafted Aggregate does not run as a template: %v", err)
	}
	return b.String()
}

// TestTheModuleFormPassesDownTheParametersItDeclares imports in the module form
// with the module's signing_key parameter passed down:
//
//   - one source carried: the drafted Aggregate declares signing_key once, under
//     spec.parameters, as a required string of at least one character with no
//     default and the description given, names it first in the comment of what
//     the Release supplies, and passes it down to the entry by name ahead of the
//     carried flow — byte for byte;
//   - one source not carried, its writer refusing: the entry passes signing_key
//     down all the same, under a parameters key of its own, with the comment
//     saying why the flow is left unset;
//   - two sources: signing_key is declared once and both entries pass it down.
//
// Evaluated as a template, as the compiler evaluates an Aggregate, with the
// Release fulfilling signing_key, every entry reads the value fulfilled and the
// description reads back as written, read as the double-quoted scalar it is
// through JSON, whose escapes are a subset of that scalar's.
//
// 🔴 PLANTED DEFECT: in moduleEntity (module.go), write the passed-down lines only
// where the flow is carried (`if inst.carried {` around the loop writing them):
// the row whose flow is not carried fails.
func TestTheModuleFormPassesDownTheParametersItDeclares(t *testing.T) {
	t.Parallel()
	escaped := strings.Replace(moduleSourceLine, "{{question}}", `{{"{{"}}question}}`, 1)
	passing := "      parameters:\n        signing_key: \"{{ .parameters.signing_key }}\"\n"
	for _, tc := range []struct {
		name     string
		settings string
		writer   *emptyRecorder
		sources  []aggrega.ImportSource
		// want is the drafted Aggregate, where it is pinned byte for byte, and
		// entries the entries that must each pass signing_key down.
		want    string
		entries []string
		contain []string
	}{
		{name: "one source carried", writer: &emptyRecorder{}, sources: []aggrega.ImportSource{oneSource(moduleSource)},
			want: passedDownHead + passedDownComment + "    testmod:\n      type: core/testmod\n" + passing +
				"        flow: |\n          " + escaped + "\n",
			entries: []string{"testmod"}},
		{name: "one source not carried", settings: `{"empty_credentials":"marked"}`, writer: &emptyRecorder{fail: true},
			sources: []aggrega.ImportSource{oneSource(moduleSource)}, entries: []string{"testmod"},
			contain: []string{passedDownHead + passedDownComment + "    testmod:\n      type: core/testmod\n" + passing +
				"      # parameters.flow is left unset: the import's diagnostics say why the flow is not carried.\n"}},
		{name: "two sources", writer: &emptyRecorder{},
			sources: []aggrega.ImportSource{{Name: "a.json", Bytes: []byte(moduleSource)},
				{Name: "b.json", Bytes: []byte(moduleSource)}},
			entries: []string{"testmod", "testmod-2"},
			contain: []string{passedDownHead + passedDownComment + "    testmod:\n      type: core/testmod\n" + passing,
				passedDownComment + "    testmod-2:\n      type: core/testmod\n" + passing}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := platformImport(t, passingDownFormat(tc.writer), platformCatalog(), tc.settings, tc.sources...)
			if len(out.Files) != 1 || out.Files[0].Path != "aggregate.yaml" {
				t.Fatalf("the module form wrote %+v, want the Aggregate alone", out.Files)
			}
			agg := out.Files[0].Content
			if tc.want != "" && agg != tc.want {
				t.Errorf("the module form drafted\n%s\nwant\n%s", agg, tc.want)
			}
			for _, want := range tc.contain {
				if !strings.Contains(agg, want) {
					t.Errorf("the draft does not hold\n%s\nit reads\n%s", want, agg)
				}
			}
			if n := strings.Count(agg, "    signing_key:\n      type: string\n"); n != 1 {
				t.Errorf("the draft declares signing_key %d times, want once:\n%s", n, agg)
			}
			if strings.Contains(agg, "default:") {
				t.Errorf("the draft gives a passed-down parameter a default:\n%s", agg)
			}

			text := evaluated(t, agg, "k-fulfilled")
			if n := strings.Count(text, "\n        signing_key: \"k-fulfilled\"\n"); n != len(tc.entries) {
				t.Errorf("evaluated, %d entries read the fulfilled signing_key, want %d (%v):\n%s", n, len(tc.entries),
					tc.entries, text)
			}
			const opening = "\n      description: "
			at := strings.Index(text, opening)
			if at < 0 {
				t.Fatalf("evaluated, the draft holds no description:\n%s", text)
			}
			line := text[at+len(opening):]
			line = line[:strings.IndexByte(line, '\n')]
			var description string
			if err := json.Unmarshal([]byte(line), &description); err != nil {
				t.Fatalf("evaluated, the description %s does not read back as a double-quoted scalar: %v", line, err)
			}
			if description != passedDownDescription {
				t.Errorf("evaluated, the description reads %q, want %q", description, passedDownDescription)
			}
		})
	}
}

// TestTheModuleFormReportsWhatItCannotDo imports the flow in the module form with
// attach_egress_allowlist true, against a catalog view holding no entry for the
// platform's Component: the flow is carried all the same, a warning says no
// Feature is attached since an entry of a composed Aggregate takes none, and
// another that the catalog holds no platform Component.
func TestTheModuleFormReportsWhatItCannotDo(t *testing.T) {
	t.Parallel()
	out := platformImport(t, moduleTestFormat(&emptyRecorder{}), standardCatalog, `{"attach_egress_allowlist":true}`,
		oneSource(moduleSource))
	if want := renderedModuleAggregate(moduleSourceLine); len(out.Files) != 1 || out.Files[0].Content != want {
		t.Errorf("the module form wrote %+v, want the Aggregate alone carrying the flow", out.Files)
	}
	var notAttached, notInCatalog string
	for _, d := range out.Diagnostics {
		switch d.Code {
		case "feature-not-attached":
			notAttached = d.Message
		case "platform-not-in-catalog":
			notInCatalog = d.Message
		}
	}
	if want := "the attach_egress_allowlist setting attaches core/egress-allowlist to the platform a draft carries " +
		"the flow in, and this draft composes core/testmod, whose entry in the drafted Aggregate takes parameters and " +
		"no Feature, so nothing is attached"; notAttached != want {
		t.Errorf("the setting is reported as %q, want %q", notAttached, want)
	}
	if want := "the workspace's catalog holds no core/ai-agent/testplat: the draft composes core/testmod, which runs " +
		"it, all the same"; notInCatalog != want {
		t.Errorf("the absent platform is reported as %q, want %q", notInCatalog, want)
	}
}
