// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package aggrega_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// TestMarshalDescriptor_OmitsAbsentOptionals pins the wire-faithful omission a
// migrated guest depends on: a CLI descriptor that declares no render-behavior
// contract marshals WITHOUT a "contract" key (and without any other absent
// optional), so the typed builder emits the same bytes the hand-written JSON
// literal did.
func TestMarshalDescriptor_OmitsAbsentOptionals(t *testing.T) {
	t.Parallel()
	d := aggrega.PluginDescriptor{
		AbiVersion: aggrega.APIVersion,
		PluginType: "cli",
		Exports:    []aggrega.PluginExport{{Name: "cli_run", HostFunctions: []string{"aggrega_cli_write_stdout"}}},
		Commands: []aggrega.PluginCLICommand{{
			Name:        "echo-args",
			Description: "Echo the received argument vector to standard output.",
			Flags: []aggrega.PluginCommandFlag{{
				Name:        "upper",
				Description: "Echo the arguments upper-cased.",
				Type:        "bool",
				Default:     "false",
			}},
		}},
		Metadata: &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
	got, err := aggrega.MarshalDescriptor(d)
	if err != nil {
		t.Fatalf("MarshalDescriptor: %v", err)
	}
	t.Logf("cli descriptor wire: %s", got)
	for _, key := range []string{`"contract"`, `"stage"`, `"settings_schema"`, `"capabilities"`, `"version"`, `"commit"`} {
		if bytes.Contains(got, []byte(key)) {
			t.Errorf("absent optional %s leaked into wire: %s", key, got)
		}
	}
	// The flag surface (commands[].flags[]) is present and carries its fields.
	for _, want := range []string{`"commands"`, `"flags"`, `"upper"`, `"bool"`} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("expected %s in wire, got: %s", want, got)
		}
	}
}

// TestMarshalDescriptor_ProviderFullContract proves a provider descriptor with
// a fully populated contract marshals the contract correctly and round-trips.
func TestMarshalDescriptor_ProviderFullContract(t *testing.T) {
	t.Parallel()
	d := aggrega.PluginDescriptor{
		AbiVersion:   aggrega.APIVersion,
		PluginType:   "provider",
		Description:  "Render one IR node and assemble the dispatch group.",
		Capabilities: []string{"helm"},
		Exports: []aggrega.PluginExport{
			{Name: "render_node", InputPattern: "b", Stability: "experimental"},
			{Name: "assemble_boundary", InputPattern: "b"},
		},
		Contract: &aggrega.ProviderContract{
			InputKinds:       aggrega.InputKinds{Sourced: true, Synthesized: true},
			BoundaryAssembly: "merge",
			ArtifactDelimiters: &aggrega.ArtifactDelimiters{
				Left:  "[[",
				Right: "]]",
			},
			AssemblyNameAnnotation: &aggrega.AssemblyNameAnnotation{
				Annotation:    "aggrega.ai/release-name",
				RequireUnique: true,
			},
			ReusableInstance: true,
		},
		Metadata: &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
	got, err := aggrega.MarshalDescriptor(d)
	if err != nil {
		t.Fatalf("MarshalDescriptor: %v", err)
	}
	t.Logf("provider descriptor wire: %s", got)
	for _, want := range []string{
		`"contract"`, `"input_kinds"`, `"boundary_assembly":"merge"`,
		`"artifact_delimiters"`, `"[["`,
		`"assembly_name_annotation"`, `"require_unique":true`, `"reusable_instance":true`,
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("expected %s in wire, got: %s", want, got)
		}
	}

	// Round-trip: decode the wire and re-encode; the bytes are stable and the
	// decoded value equals the original.
	var round aggrega.PluginDescriptor
	if err := round.UnmarshalJSON(got); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !reflect.DeepEqual(d, round) {
		t.Errorf("round-trip inequality:\n  before: %+v\n  after:  %+v", d, round)
	}
	got2, err := aggrega.MarshalDescriptor(round)
	if err != nil {
		t.Fatalf("MarshalDescriptor (re-encode): %v", err)
	}
	if !bytes.Equal(got, got2) {
		t.Errorf("re-encode not byte-stable:\n first: %s\n  second: %s", got, got2)
	}
}

// TestMarshalDescriptor_MetadataAppend proves the build-provenance stamp folds
// the Version and Commit package vars into the descriptor's metadata, and that
// an unstamped build omits both — the typed replacement for the string-concat
// metadata-append the core plugins used.
func TestMarshalDescriptor_MetadataAppend(t *testing.T) {
	// Version and Commit are package-level vars mutated here, so this test does
	// not run in parallel with others that read the descriptor surface.
	origVersion, origCommit := aggrega.Version, aggrega.Commit
	t.Cleanup(func() { aggrega.Version, aggrega.Commit = origVersion, origCommit })

	base := func() aggrega.PluginDescriptor {
		return aggrega.PluginDescriptor{
			AbiVersion: aggrega.APIVersion,
			PluginType: "provider",
			Exports:    []aggrega.PluginExport{{Name: "render_node", InputPattern: "b"}},
			Metadata:   &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
		}
	}

	t.Run("unstamped omits version and commit", func(t *testing.T) {
		aggrega.Version, aggrega.Commit = "", ""
		got, err := aggrega.MarshalDescriptor(base())
		if err != nil {
			t.Fatalf("MarshalDescriptor: %v", err)
		}
		for _, key := range []string{`"version"`, `"commit"`} {
			if bytes.Contains(got, []byte(key)) {
				t.Errorf("unstamped build leaked %s: %s", key, got)
			}
		}
	})

	t.Run("stamped folds version and commit into metadata", func(t *testing.T) {
		aggrega.Version, aggrega.Commit = "1.2.3", "abc1234"
		got, err := aggrega.MarshalDescriptor(base())
		if err != nil {
			t.Fatalf("MarshalDescriptor: %v", err)
		}
		t.Logf("stamped wire: %s", got)
		for _, want := range []string{`"version":"1.2.3"`, `"commit":"abc1234"`, `"author":"aggrega"`, `"license":"Apache-2.0"`} {
			if !bytes.Contains(got, []byte(want)) {
				t.Errorf("expected %s in stamped wire, got: %s", want, got)
			}
		}
	})

	t.Run("stamp allocates metadata when the descriptor declared none", func(t *testing.T) {
		aggrega.Version, aggrega.Commit = "9.9.9", ""
		d := aggrega.PluginDescriptor{
			AbiVersion: aggrega.APIVersion,
			PluginType: "cli",
			Exports:    []aggrega.PluginExport{{Name: "cli_run"}},
		}
		got, err := aggrega.MarshalDescriptor(d)
		if err != nil {
			t.Fatalf("MarshalDescriptor: %v", err)
		}
		if !bytes.Contains(got, []byte(`"metadata":{"version":"9.9.9"}`)) {
			t.Errorf("expected allocated metadata with version, got: %s", got)
		}
		if bytes.Contains(got, []byte(`"commit"`)) {
			t.Errorf("commit should be absent when unset: %s", got)
		}
	})
}
