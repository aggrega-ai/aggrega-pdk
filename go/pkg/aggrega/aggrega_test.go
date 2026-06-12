// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package aggrega_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// loadSchema reads the JSON Schema at the given relative path
// (resolved from this test file's directory) and compiles a
// jsonschema.Schema for validation. Returns nil after a t.Skipf when
// the file is not reachable, so the test suite stays clean on
// consumer checkouts that do not vendor the schemas alongside.
func loadSchema(t *testing.T, schemaFile string) *jsonschema.Schema {
	t.Helper()
	schemaPath := filepath.Join("..", "..", "..", "..", "aggrega-schemas", "json", schemaFile)
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Skipf("schema not reachable at %s: %v", schemaPath, err)
	}
	var raw any
	if err := json.Unmarshal(schemaBytes, &raw); err != nil {
		t.Fatalf("unmarshal schema %s: %v", schemaFile, err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(schemaFile, raw); err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	schema, err := c.Compile(schemaFile)
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

func TestMutatorInput_RoundTripJSONSchema(t *testing.T) {
	t.Parallel()
	schema := loadSchema(t, "wasm-mutator-v1alpha1.schema.json")

	fixture := aggrega.MutatorInput{
		CompilationContext: aggrega.CompilationContext{
			ReleaseDestination: "aws-prod/eks-main",
			ReleaseName:        "payments",
			Environment:        "prod",
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

func TestPolicyValidatorInput_RoundTripJSONSchema(t *testing.T) {
	t.Parallel()
	schema := loadSchema(t, "wasm-policy-validator-v1alpha1.schema.json")

	// Fixture exercises Features attachments — verifies the PDK's
	// PolicyFeatureEntry shape serialises to JSON that the schema's
	// PolicyFeatureEntry definition accepts.
	fixture := aggrega.PolicyValidatorInput{
		Graph: aggrega.PolicyGraph{
			Nodes: map[string]aggrega.PolicyNode{
				"payments-api": {
					Component:       "core/web-service",
					ResolvedVersion: "1.4.2",
					Labels:          map[string]string{"team": "payments"},
					Features: map[string]aggrega.PolicyFeatureEntry{
						"observability": {
							Type:            "feature/observability",
							ResolvedVersion: "2.0.1",
							Values:          map[string]any{"tracing_enabled": true},
							Origin:          "explicit",
						},
					},
				},
			},
			Edges: []aggrega.PolicyEdge{
				{Source: "payments-api", Target: "payments-db", Socket: "databases"},
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
		t.Errorf("schema validation failed for PolicyValidatorInput fixture: %v", err)
	}
}

func TestPolicyValidatorOutput_RoundTripJSONSchema(t *testing.T) {
	t.Parallel()
	schema := loadSchema(t, "wasm-policy-validator-v1alpha1.schema.json")

	fixture := aggrega.PolicyValidatorOutput{
		Results: []aggrega.PolicyResult{
			{Pass: true, Message: "all labels present"},
			{Pass: false, Message: "version floor violation: payments-db < 15.0.0"},
		},
	}

	if err := schema.Validate(jsonInstance(t, fixture)); err != nil {
		t.Errorf("schema validation failed for PolicyValidatorOutput fixture: %v", err)
	}

	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var round aggrega.PolicyValidatorOutput
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(fixture, round) {
		t.Errorf("round-trip inequality:\n  before: %+v\n  after:  %+v", fixture, round)
	}
}
