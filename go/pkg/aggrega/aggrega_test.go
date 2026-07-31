// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package aggrega_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

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
// Returns nil after a t.Skipf when the directory or one of its schemas is not
// reachable, so the test suite stays clean on consumer checkouts that do not
// vendor the schemas alongside.
func loadSchema(t *testing.T, schemaFile string) *jsonschema.Schema {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "aggrega-schemas", "json")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("schemas not reachable at %s: %v", dir, err)
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
			t.Skipf("schema not reachable at %s: %v", schemaPath, err)
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
	schema, err := c.Compile(id)
	if err != nil {
		t.Fatalf("Compile schema %s: %v", schemaFile, err)
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

func TestAPIVersion_v1alpha1ContractFloor(t *testing.T) {
	t.Parallel()
	if aggrega.APIVersion != "v1alpha1" {
		t.Errorf("APIVersion = %q, want %q", aggrega.APIVersion, "v1alpha1")
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

	fixture := aggrega.MutatorInput{
		CompilationContext: aggrega.CompilationContext{
			ReleaseName: "payments",
			Environment: "prod",
		},
		Graph: aggrega.MutatorGraph{
			Nodes: map[string]aggrega.MutatorNode{
				"payments-api": {
					Component:       "core/web-service",
					ResolvedVersion: "1.4.2",
					Labels:          map[string]string{"team": "payments"},
				},
			},
		},
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
	// from ir-v1alpha1). The guest IR types use plain maps (the guest reads a
	// map; key order on the wire is the host's concern).
	fixture := aggrega.PolicyInput{
		Graph: aggrega.IRGraph{
			APIVersion: "aggrega.ai/v1alpha1",
			Kind:       "IR",
			Metadata:   aggrega.IRMetadata{AggregateName: "payments"},
			Spec: aggrega.IRSpec{
				Nodes: map[string]aggrega.IRNode{
					"payments-api": {
						Component:       "core/web-service",
						ResolvedVersion: "1.4.2",
						Labels:          map[string]string{"team": "payments"},
					},
				},
				Edges: []aggrega.IREdge{{
					Source: "payments-api", Target: "payments-db", Socket: "databases",
					SocketDeclaration: "declared",
				}},
				FeatureMarkers:       []string{},
				ValuesMapping:        map[string]json.RawMessage{},
				InfraMapping:         map[string]json.RawMessage{},
				FeatureValuesMapping: map[string]map[string]aggrega.IRFeatureEntry{},
				Provenance: aggrega.IRProvenance{
					SchemaVersion: "v1alpha1", CompilerVersion: "test", AggregateVersion: "1.0.0",
					ManifestVersion: "1.0.0", ReleaseVersion: "1.0.0", SourceIdentifiers: []string{"x"},
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
			{Severity: "deny", Message: "version floor violation: payments-db < 15.0.0"},
			{Severity: "audit", Message: "all labels present"},
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
