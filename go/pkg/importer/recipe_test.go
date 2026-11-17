// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func goldenRecipeDraft() RecipeDraft {
	return RecipeDraft{
		Name:        "bucket-acme",
		Version:     "0.1.0",
		Description: "A bucket drafted as a Terraform module.",
		Capability:  "terraform",
		Implements:  "acme/resource/bucket:0.1.0",
		Header:      []string{"Drafted from a vendor schema; review before use."},
		Source:      "./module",
		Locals: []Entry{
			{Key: "logical_id", Value: "{{ capitalizedWords .context.instance_id }}", Comment: []string{"The resource's logical id."}},
		},
		Provides: []Entry{
			{Key: "arn", Value: "{{ .locals.logical_id }}.arn"},
		},
		Assertions: []Entry{
			{Key: "name_required", Value: "{{ if not .values.name }}values.name is required{{ end }}"},
		},
		Artifacts: []Entry{
			{Key: "main.tf.json", Value: "{{ toJson .values.main }}"},
			{Key: "notes.txt", Value: "first line\nsecond line\n"},
		},
		BoundaryArtifacts: []Entry{
			{Key: "provider.tf", Value: "provider \"acme\" {\n  features {}\n}\n", Comment: []string{"Read once for the whole boundary."}},
		},
		ValuesMapping: "name: {{ toJson .values.name }}\nversioning: {{ toJson .values.versioning }}\n",
		Files: []RecipeFile{
			{Path: "module/main.tf.json", Content: "{\"resource\":{}}\n"},
		},
	}
}

// TestDraftRecipeGolden holds a draft carrying every part and a file beside it
// to the file in testdata/recipe.
func TestDraftRecipeGolden(t *testing.T) {
	got := DraftRecipe(goldenRecipeDraft())
	want, err := os.ReadFile(filepath.Join("testdata", "recipe", "golden.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Entity.Body != string(want) {
		t.Errorf("DraftRecipe body =\n%s\nwant\n%s", got.Entity.Body, want)
	}
	if got.Entity.Kind != "Recipe" || got.Entity.Mode != "draft" || got.Entity.Name != "bucket-acme" {
		t.Errorf("entity = %+v", got.Entity)
	}
	if got.Version != "0.1.0" || got.Capability != "terraform" {
		t.Errorf("Version = %q, Capability = %q", got.Version, got.Capability)
	}
	if !reflect.DeepEqual(got.Files, goldenRecipeDraft().Files) {
		t.Errorf("Files = %+v", got.Files)
	}
	var doc struct {
		Spec struct {
			Locals    map[string]string `yaml:"locals"`
			Artifacts map[string]string `yaml:"artifacts"`
			Boundary  struct {
				Artifacts map[string]string `yaml:"artifacts"`
			} `yaml:"boundary"`
			ValuesMapping string `yaml:"valuesMapping"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal([]byte(got.Entity.Body), &doc); err != nil {
		t.Fatal(err)
	}
	g := goldenRecipeDraft()
	if doc.Spec.Locals["logical_id"] != g.Locals[0].Value || doc.Spec.Artifacts["notes.txt"] != g.Artifacts[1].Value ||
		doc.Spec.Boundary.Artifacts["provider.tf"] != g.BoundaryArtifacts[0].Value || doc.Spec.ValuesMapping != g.ValuesMapping {
		t.Errorf("values do not read back as written: %+v", doc.Spec)
	}
}

// TestDraftRecipeOmitsEmptyParts holds a draft of the required fields alone to
// an empty spec.
func TestDraftRecipeOmitsEmptyParts(t *testing.T) {
	got := DraftRecipe(RecipeDraft{Name: "r", Version: "1.0.0", Capability: "terraform", Implements: "a/b/c:1.0.0"}).Entity.Body
	want := "apiVersion: aggrega.ai/v1beta1\nkind: Recipe\nmetadata:\n  name: r\n  version: 1.0.0\n" +
		"spec:\n  capability: terraform\n  implements: a/b/c:1.0.0\n"
	if got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
	bare := DraftRecipe(RecipeDraft{Name: "r", Version: "1.0.0"}).Entity.Body
	if want := "apiVersion: aggrega.ai/v1beta1\nkind: Recipe\nmetadata:\n  name: r\n  version: 1.0.0\nspec: {}\n"; bare != want {
		t.Errorf("bare body =\n%s\nwant\n%s", bare, want)
	}
}

// TestDraftRecipeWritesHostileText reads hostile text written as a header line,
// an entry comment and an entry value back through a YAML reader.
func TestDraftRecipeWritesHostileText(t *testing.T) {
	values := []string{
		"uses {{ braces", "a # not a comment", "[vendor schema text] leading bracket", "---", "key: value",
		"carriage\rreturn", "trailing backslash\\", "two\nlines", "two\nlines\n", "\nleading break", "  leading space\nx",
		"tab\there", "\"quoted\"", "ünïcode ☃ 😀", "", "ctl\x01char", "nel\u0085x\nmore", "\n", "a\n\n\n",
	}
	for _, v := range values {
		d := RecipeDraft{
			Name: "r", Version: "1.0.0", Description: v, Capability: "terraform", Implements: "a/b/c:1.0.0",
			Header:            []string{v, "after: " + v},
			Locals:            []Entry{{Key: "k", Value: v, Comment: []string{v}}},
			Artifacts:         []Entry{{Key: v + "key", Value: v, Comment: []string{v}}},
			BoundaryArtifacts: []Entry{{Key: "b", Value: v, Comment: []string{v}}},
			ValuesMapping:     v,
		}
		body := DraftRecipe(d).Entity.Body
		var doc struct {
			Metadata struct {
				Description string `yaml:"description"`
			} `yaml:"metadata"`
			Spec struct {
				Capability    string            `yaml:"capability"`
				Locals        map[string]string `yaml:"locals"`
				Artifacts     map[string]string `yaml:"artifacts"`
				ValuesMapping string            `yaml:"valuesMapping"`
				Boundary      struct {
					Artifacts map[string]string `yaml:"artifacts"`
				} `yaml:"boundary"`
			} `yaml:"spec"`
		}
		if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
			t.Errorf("%q: %v\n%s", v, err, body)
			continue
		}
		if doc.Metadata.Description != v {
			t.Errorf("description %q read back as %q", v, doc.Metadata.Description)
		}
		if doc.Spec.Capability != "terraform" || len(doc.Spec.Locals) != 1 || len(doc.Spec.Artifacts) != 1 {
			t.Errorf("%q: comment text changed the document:\n%s", v, body)
		}
		if got := doc.Spec.Locals["k"]; got != v {
			t.Errorf("local %q read back as %q", v, got)
		}
		if got := doc.Spec.Artifacts[v+"key"]; got != v {
			t.Errorf("artifact %q read back as %q", v, got)
		}
		if got := doc.Spec.Boundary.Artifacts["b"]; got != v {
			t.Errorf("boundary artifact %q read back as %q", v, got)
		}
		if got := doc.Spec.ValuesMapping; got != v && v != "" {
			t.Errorf("valuesMapping %q read back as %q", v, got)
		}
		if strings.Contains(body, "{{ braces") && !strings.Contains(body, "{{`{{`}}") {
			t.Errorf("a comment is written without the template escape:\n%s", body)
		}
	}
}

// TestDraftRecipeIsDeterministic holds two drafts of one input to equal bytes.
func TestDraftRecipeIsDeterministic(t *testing.T) {
	a, b := DraftRecipe(goldenRecipeDraft()), DraftRecipe(goldenRecipeDraft())
	if a.Entity.Body != b.Entity.Body {
		t.Errorf("bodies differ:\n%s\n%s", a.Entity.Body, b.Entity.Body)
	}
}
