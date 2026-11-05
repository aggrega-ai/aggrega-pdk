// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package aggrega_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// skipSchemaTestsEnv names the environment switch that opts out of the tests
// which validate against the JSON Schemas published alongside this kit. Set it
// to any non-empty value where those schemas are genuinely absent — a
// standalone checkout of this kit, or a packaging test — and each affected test
// reports that it was skipped and why.
const skipSchemaTestsEnv = "AGGREGA_SKIP_SCHEMA_TESTS"

// refuseOrSkipUnreachableSchemas ends the calling test when the JSON Schemas
// these tests validate against cannot be read.
//
// The default is refusal. A harness that could not reach its inputs has
// verified nothing, so reporting that is strictly more informative than passing
// quietly — and a suite that skips by default cannot tell "everything agrees"
// apart from "nothing was checked".
//
// Setting skipSchemaTestsEnv turns the refusal into a skip that states the
// reason it was taken, so a deliberate run without the schemas stays clean
// while an accidental one stays loud.
//
// This is the one place that decides between the two, so every call site
// reaches the same verdict on the same condition.
func refuseOrSkipUnreachableSchemas(t *testing.T, path string, err error) {
	t.Helper()
	if os.Getenv(skipSchemaTestsEnv) != "" {
		t.Skipf("skipped: schemas not reachable at %s (%v); %s is set", path, err, skipSchemaTestsEnv)
	}
	t.Fatalf("schemas not reachable at %s: %v; these tests validate against the JSON Schemas "+
		"published alongside this kit and verify nothing without them. Check out the "+
		"aggrega-schemas repository beside this one (aggrega-pdk/ and aggrega-schemas/ as "+
		"siblings), or set %s=1 to skip them.",
		path, err, skipSchemaTestsEnv)
}

// loadSchema compiles the named JSON Schema for validation, with every schema
// in the sibling aggrega-schemas json directory registered alongside it. The
// registration set is derived by reading that directory, so a cross-file $ref
// resolves locally without a network loader and without any per-caller list of
// referenced schemas to keep in step with the schemas themselves.
//
// Each schema registers under its $id when it declares one, falling back to its
// file name: a relative cross-file $ref resolves against the referring schema's
// base URI, which is its absolute $id URL, so the target must be reachable under
// that URL rather than under its file name. The named schema is compiled by the
// same identity it was registered under.
//
// Does not return when the directory or one of its schemas is unreachable: the
// calling test is refused, or skipped with its reason stated under the opt-out
// switch.
func loadSchema(t *testing.T, schemaFile string) *jsonschema.Schema {
	t.Helper()
	return compileSchemaAt(t, schemaFile, "")
}

// schemaDir is the sibling aggrega-schemas repository's json directory.
var schemaDir = filepath.Join("..", "..", "..", "..", "aggrega-schemas", "json")

// exportTableKeyword is the top-level keyword under which a socket schema maps
// each export the host calls a guest with to the definition of its input and the
// definition of its output, each a JSON Pointer into the schema's own $defs.
const exportTableKeyword = "x-aggrega-exports"

// loadExportPayloadSchema compiles the definition a socket schema's export table
// names for one export's input or output — direction is "input" or "output".
//
// A socket schema declares no shape at its root, because one document can be a
// valid payload of several exports — an empty object answers more than one of the
// provider's — so a payload is validated against the definition its export names.
// The definition is read from the table rather than named here, so these tests
// validate against exactly what the schema publishes for the export.
//
// Does not return when the schema is unreachable, matching loadSchema, or when
// the table does not map the export in that direction.
func loadExportPayloadSchema(t *testing.T, schemaFile, export, direction string) *jsonschema.Schema {
	t.Helper()
	return compileSchemaAt(t, schemaFile, exportTablePointer(t, schemaFile, export, direction))
}

// exportTablePointer reads the pointer a socket schema's export table holds for
// one export's input or output.
func exportTablePointer(t *testing.T, schemaFile, export, direction string) string {
	t.Helper()
	table := exportTable(t, schemaFile)
	row, ok := table[export]
	if !ok {
		t.Fatalf("%s's %s table maps no export %q", schemaFile, exportTableKeyword, export)
	}
	pointer, ok := row[direction]
	if !ok || pointer == "" {
		t.Fatalf("%s's %s table names no %s definition for export %q", schemaFile, exportTableKeyword, direction, export)
	}
	return pointer
}

// exportTable reads a socket schema's export table: export name to direction to
// pointer.
func exportTable(t *testing.T, schemaFile string) map[string]map[string]string {
	t.Helper()
	path := filepath.Join(schemaDir, schemaFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		refuseOrSkipUnreachableSchemas(t, path, err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal schema %s: %v", schemaFile, err)
	}
	var table map[string]map[string]string
	if err := json.Unmarshal(doc[exportTableKeyword], &table); err != nil || len(table) == 0 {
		t.Fatalf("%s carries no readable %s table: %v", schemaFile, exportTableKeyword, err)
	}
	return table
}

// compileSchemaAt compiles the named schema, or the location fragment names
// inside it, with every schema in the sibling json directory registered alongside
// it, as loadSchema describes.
func compileSchemaAt(t *testing.T, schemaFile, fragment string) *jsonschema.Schema {
	t.Helper()
	dir := schemaDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		refuseOrSkipUnreachableSchemas(t, dir, err)
	}
	c := jsonschema.NewCompiler()
	ids := make(map[string]string, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".schema.json") {
			continue
		}
		schemaPath := filepath.Join(dir, name)
		schemaBytes, err := os.ReadFile(schemaPath)
		if err != nil {
			refuseOrSkipUnreachableSchemas(t, schemaPath, err)
		}
		var raw any
		if err := json.Unmarshal(schemaBytes, &raw); err != nil {
			t.Fatalf("unmarshal schema %s: %v", name, err)
		}
		id := name
		if obj, ok := raw.(map[string]any); ok {
			if s, ok := obj["$id"].(string); ok && s != "" {
				id = s
			}
		}
		if err := c.AddResource(id, raw); err != nil {
			t.Fatalf("AddResource %s: %v", id, err)
		}
		ids[name] = id
	}
	id, ok := ids[schemaFile]
	if !ok {
		t.Fatalf("schema %s not found in %s", schemaFile, dir)
	}
	schema, err := c.Compile(id + fragment)
	if err != nil {
		t.Fatalf("Compile schema %s%s: %v", schemaFile, fragment, err)
	}
	return schema
}

// jsonInstance marshals v and unmarshals back to a generic any value
// the jsonschema validator consumes.
func jsonInstance(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var inst any
	if err := json.Unmarshal(raw, &inst); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return inst
}

// schemaDeclaredABIVersion reads the plugin ABI version out of the descriptor
// schema in the sibling aggrega-schemas repository — the single-valued const the
// schema declares for a descriptor's abi_version. Reading the declaration rather
// than repeating its value is the whole point: a literal here would pin this
// kit's constant against a second copy living in this same repository, which is
// the divergence that generating the constant from the schema exists to remove.
//
// Does not return when the schema is unreachable, matching loadSchema: the
// calling test is refused, or skipped with its reason stated under the opt-out
// switch.
func schemaDeclaredABIVersion(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "aggrega-schemas", "json",
		"wasm-describe-v1alpha1.schema.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		refuseOrSkipUnreachableSchemas(t, path, err)
	}
	var doc struct {
		Defs struct {
			PluginDescriptor struct {
				Properties struct {
					AbiVersion struct {
						Const string `json:"const"`
					} `json:"abi_version"`
				} `json:"properties"`
			} `json:"PluginDescriptor"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	got := doc.Defs.PluginDescriptor.Properties.AbiVersion.Const
	// An empty read means the declaration moved, not that the two versions agree.
	// Failing here keeps a silent zero from being reported as a match.
	if got == "" {
		t.Fatalf("no abi_version const found at $defs.PluginDescriptor.properties in %s: "+
			"the declaration moved, so this assertion is no longer reading it", path)
	}
	return got
}

// TestAPIVersion_MatchesSchemaDeclaration checks this kit's exported constant
// against the schema that declares the version, which is the one artifact
// upstream of every language's copy.
func TestAPIVersion_MatchesSchemaDeclaration(t *testing.T) {
	t.Parallel()
	want := schemaDeclaredABIVersion(t)
	if aggrega.APIVersion != want {
		t.Errorf("APIVersion = %q but the descriptor schema declares abi_version %q; "+
			"this constant is generated from that declaration, so a mismatch means the "+
			"generated const is stale — regenerate it", aggrega.APIVersion, want)
	}
}

func TestCLIRunInput_RoundTripJSONSchema(t *testing.T) {
	t.Parallel()
	schema := loadSchema(t, "wasm-cli-v1alpha1.schema.json")

	fixture := aggrega.CLIRunInput{Command: "deploy", Args: []string{"deploy", "--env", "prod", "payments"}}

	if err := schema.Validate(jsonInstance(t, fixture)); err != nil {
		t.Errorf("schema validation failed for CLIRunInput fixture: %v", err)
	}

	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var round aggrega.CLIRunInput
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(fixture, round) {
		t.Errorf("round-trip inequality:\n  before: %+v\n  after:  %+v", fixture, round)
	}
}

func TestMutatorInput_RoundTripJSONSchema(t *testing.T) {
	t.Parallel()
	schema := loadSchema(t, "wasm-mutator-v1alpha1.schema.json")

	var labels aggrega.OrderedMap[string]
	labels.Set("team", "payments")
	var nodes aggrega.OrderedMap[aggrega.MutatorNode]
	nodes.Set("payments-api", aggrega.MutatorNode{
		Component:       "core/web-service",
		ResolvedVersion: "1.4.2",
		Labels:          labels,
	})
	fixture := aggrega.MutatorInput{
		CompilationContext: aggrega.CompilationContext{
			ReleaseName: "payments",
			Environment: "prod",
		},
		Graph: aggrega.MutatorGraph{Nodes: nodes},
	}

	if err := schema.Validate(jsonInstance(t, fixture)); err != nil {
		t.Errorf("schema validation failed for MutatorInput fixture: %v", err)
	}

	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var round aggrega.MutatorInput
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(fixture, round) {
		t.Errorf("round-trip inequality:\n  before: %+v\n  after:  %+v", fixture, round)
	}
}

func TestMutatorOutput_RoundTripJSONSchema(t *testing.T) {
	t.Parallel()
	schema := loadSchema(t, "wasm-mutator-v1alpha1.schema.json")

	patch := json.RawMessage(`[{"op":"add","path":"/nodes/payments-api/labels/compliance","value":"soc2"}]`)
	fixture := aggrega.MutatorOutput{
		Patches: []json.RawMessage{patch},
	}

	if err := schema.Validate(jsonInstance(t, fixture)); err != nil {
		t.Errorf("schema validation failed for MutatorOutput fixture: %v", err)
	}
}

func TestPolicyInput_RoundTripJSONSchema(t *testing.T) {
	t.Parallel()
	// The policy schema references ir-v1alpha1 for its graph field and
	// wasm-diagnostic-v1alpha1 for its result entries; both resolve because the
	// whole schema directory is registered with the compiler.
	schema := loadSchema(t, "wasm-policy-v1alpha1.schema.json")

	// Under b-full the policy input carries the full IR graph (single-sourced
	// from ir-v1alpha1). Every map the IR schema marks order-preserving is an
	// aggrega.OrderedMap, which keeps the order its entries were set in.
	var labels aggrega.OrderedMap[string]
	labels.Set("team", "payments")
	var nodes aggrega.OrderedMap[aggrega.IRNode]
	nodes.Set("payments-api", aggrega.IRNode{
		Component:       "core/web-service",
		ResolvedVersion: "1.4.2",
		Labels:          labels,
	})
	fixture := aggrega.PolicyInput{
		Graph: aggrega.IRGraph{
			APIVersion: "aggrega.ai/v1alpha1",
			Kind:       "IR",
			Metadata:   aggrega.IRMetadata{AggregateName: "payments"},
			Spec: aggrega.IRSpec{
				Nodes: nodes,
				Edges: []aggrega.IREdge{{
					Source: "payments-api", Target: "payments-db", Socket: "databases",
					SocketDeclaration: "declared",
				}},
				Provenance: aggrega.IRProvenance{
					SchemaVersion: "v1alpha1", CompilerVersion: "test", AggregateVersion: "1.0.0",
					ManifestVersion: "1.0.0", ReleaseVersion: "1.0.0",
				},
			},
		},
		PrecalculatedPaths: []aggrega.PrecalculatedPath{
			{
				PathID:           "path-0001",
				Nodes:            []string{"payments-api", "payments-db"},
				AttachedFeatures: []string{"feature/observability"},
			},
		},
	}

	if err := schema.Validate(jsonInstance(t, fixture)); err != nil {
		t.Errorf("schema validation failed for PolicyInput fixture: %v", err)
	}
}

func TestPolicyOutput_RoundTripJSONSchema(t *testing.T) {
	t.Parallel()
	// Compiling the policy schema resolves its graph $ref into ir-v1alpha1 even
	// though the output payload omits the graph, so the cross-file targets must
	// be registered here too.
	schema := loadSchema(t, "wasm-policy-v1alpha1.schema.json")

	fixture := aggrega.PolicyOutput{
		Results: []aggrega.Diagnostic{
			{Severity: "halt", Message: "version floor violation: payments-db < 15.0.0"},
			{Severity: "info", Message: "all labels present"},
		},
	}

	if err := schema.Validate(jsonInstance(t, fixture)); err != nil {
		t.Errorf("schema validation failed for PolicyOutput fixture: %v", err)
	}

	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var round aggrega.PolicyOutput
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(fixture, round) {
		t.Errorf("round-trip inequality:\n  before: %+v\n  after:  %+v", fixture, round)
	}
}

// TestNameBoundOutput_KeepsAnAbsentBoundDistinctFromZero asserts the three
// states a verdict's bound can be in survive the wire as three states.
//
// The distinction is why the field is a pointer rather than an integer with a
// sentinel. Absent says length is not what the target objected to — either it
// imposes none, or the objection is one no shortening repairs. Zero says no
// identity of any length reaches an acceptable name of that kind. A host unable
// to tell them apart would narrow a name toward a bound that was never the
// obstacle, and keep narrowing until it ran out of passes.
//
// The assertion reads the emitted TEXT and not only the decoded value, because
// the emitted text is where the two would collapse: a bound written as a plain
// integer emits 0 for the absent case, and nothing downstream recovers the
// difference.
func TestNameBoundOutput_KeepsAnAbsentBoundDistinctFromZero(t *testing.T) {
	t.Parallel()
	schema := loadExportPayloadSchema(t, "wasm-provider-v1alpha1.schema.json", "name_bound", "output")

	shortened := 31
	nothingFits := 0
	fixture := aggrega.NameBoundOutput{
		Names: []aggrega.DerivedNameVerdict{
			{Kind: "release name", Usable: false, Bound: &shortened, Reason: "a release name is at most 53 characters"},
			{Kind: "logical id", Usable: false, Bound: &nothingFits, Reason: "this target accepts no name of this kind"},
			{Kind: "service name", Usable: false, Reason: "the first character is one the alphabet does not admit"},
		},
	}

	if err := schema.Validate(jsonInstance(t, fixture)); err != nil {
		t.Errorf("schema validation failed for NameBoundOutput fixture: %v", err)
	}

	raw, err := fixture.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if !strings.Contains(string(raw), `"bound":0`) {
		t.Errorf("a zero bound must reach the wire as a stated length; got %s", raw)
	}

	var round aggrega.NameBoundOutput
	if err := round.UnmarshalJSON(raw); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if round.Names[0].Bound == nil || *round.Names[0].Bound != 31 {
		t.Errorf("a stated bound did not survive the round trip: %v", round.Names[0].Bound)
	}
	if round.Names[1].Bound == nil || *round.Names[1].Bound != 0 {
		t.Errorf("a zero bound must survive as a stated length, not as absence: %v", round.Names[1].Bound)
	}
	if round.Names[2].Bound != nil {
		t.Errorf("an absent bound must survive as absent, not as zero: %v", *round.Names[2].Bound)
	}
}

// TestNameBoundInput_RoundTripJSONSchema pins the question shape a provider
// guest reads: the proposed identity, and whether a person wrote it. The flag is
// carried so a guest can phrase its reason for the reader who can act on it;
// what a refusal DOES is the host's decision, not the guest's. It also carries the
// facts of the node's resolved workload some of a target's names depend on: the
// form and the Service keys, in the order the workload declares them.
func TestNameBoundInput_RoundTripJSONSchema(t *testing.T) {
	t.Parallel()
	schema := loadExportPayloadSchema(t, "wasm-provider-v1alpha1.schema.json", "name_bound", "input")

	fixture := aggrega.NameBoundInput{
		Proposed:     "payments-api",
		Authored:     true,
		WorkloadForm: "scheduled",
		ServiceKeys:  []string{"web", "metrics"},
	}

	if err := schema.Validate(jsonInstance(t, fixture)); err != nil {
		t.Errorf("schema validation failed for NameBoundInput fixture: %v", err)
	}

	raw, err := fixture.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var round aggrega.NameBoundInput
	if err := round.UnmarshalJSON(raw); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !reflect.DeepEqual(fixture, round) {
		t.Errorf("round-trip inequality:\n  before: %+v\n  after:  %+v", fixture, round)
	}
}

// TestRenderNodeOutput_EveryRenderFactValueIsAList holds what the published schema
// says about a render fact's value: it is a list, even with one entry. A render
// output whose facts are all lists validates; the same output with one value of
// any other kind does not. The host refuses such a fact when a guest returns it, so
// a provider author validating against the schema learns of it before the host
// does.
func TestRenderNodeOutput_EveryRenderFactValueIsAList(t *testing.T) {
	t.Parallel()
	schema := loadExportPayloadSchema(t, "wasm-provider-v1alpha1.schema.json", "render_node", "output")

	output := func(value string) aggrega.RenderNodeOutput {
		return aggrega.RenderNodeOutput{
			Files: []aggrega.RenderedFile{{Path: "out.txt", Content: []byte("ok\n")}},
			RenderFacts: []aggrega.RenderFact{
				{Name: "emitted_paths", Value: json.RawMessage(`["out.txt"]`)},
				{Name: "pod_labels", Value: json.RawMessage(value)},
			},
		}
	}

	if err := schema.Validate(jsonInstance(t, output(`[{"app":"api"}]`))); err != nil {
		t.Fatalf("a render output whose every fact is a list failed validation: %v", err)
	}
	for _, value := range []string{`{"app":"api"}`, `"api"`, `3`, `true`, `null`} {
		if err := schema.Validate(jsonInstance(t, output(value))); err == nil {
			t.Errorf("a render fact whose value is %s validated; the schema must declare every value a list", value)
		}
	}
}

// TestProviderExportTable_EachPayloadValidatesAgainstItsExportsDefinition
// compiles both definitions of every row of the provider schema's export table,
// so a row naming a definition the schema does not declare fails here. It then
// validates the payloads one shared root shape cannot accept — an empty object,
// which is a valid answer from several exports, and a carve input, which is also
// shaped like a reassemble input — against the definition their export names,
// each beside a definition that refuses the same document, so an acceptance
// cannot come from a definition that accepts anything.
func TestProviderExportTable_EachPayloadValidatesAgainstItsExportsDefinition(t *testing.T) {
	t.Parallel()
	const file = "wasm-provider-v1alpha1.schema.json"

	table := exportTable(t, file)
	exports := make([]string, 0, len(table))
	for export := range table {
		exports = append(exports, export)
	}
	sort.Strings(exports)
	for _, export := range exports {
		for _, direction := range []string{"input", "output"} {
			loadExportPayloadSchema(t, file, export, direction)
		}
	}

	carveInput := `{"name":"templates/deployment.yaml","body":"a2luZDogRGVwbG95bWVudAo="}`
	cases := []struct {
		name      string
		payload   string
		export    string
		direction string
		valid     bool
	}{
		{"an empty validate_node_input answer", `{}`, "validate_node_input", "output", true},
		{"an empty name_bound answer", `{}`, "name_bound", "output", true},
		{"an empty rendered_name answer", `{}`, "rendered_name", "output", true},
		{"an empty render_node answer, which carries files", `{}`, "render_node", "output", false},
		{"a carve input", carveInput, "carve", "input", true},
		{"a carve input as reassemble's answer", carveInput, "reassemble", "output", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var instance any
			if err := json.Unmarshal([]byte(tc.payload), &instance); err != nil {
				t.Fatalf("fixture %s does not parse: %v", tc.payload, err)
			}
			err := loadExportPayloadSchema(t, file, tc.export, tc.direction).Validate(instance)
			switch {
			case tc.valid && err != nil:
				t.Errorf("%s does not validate as %s's %s: %v", tc.payload, tc.export, tc.direction, err)
			case !tc.valid && err == nil:
				t.Errorf("%s validated as %s's %s, which the definition must refuse", tc.payload, tc.export, tc.direction)
			}
		})
	}
}
