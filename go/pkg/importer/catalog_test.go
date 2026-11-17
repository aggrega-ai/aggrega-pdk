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

func catalogTestDrafts() []CatalogDraft {
	c := DraftComponent(ComponentDraft{Name: "acme/resource/bucket", Version: "0.1.0"})
	c.Entity.DecisionRef = "decision:n1"
	r := DraftRecipe(goldenRecipeDraft())
	return []CatalogDraft{c, r}
}

func catalogTestDecisions(tier string) []aggrega.ImportDecision {
	return []aggrega.ImportDecision{{
		SourceNode: aggrega.DecisionSourceNode{ID: "n1"},
		Chosen:     &aggrega.DecisionChoice{Target: "acme/resource/bucket", Mode: "draft", Tier: tier},
		SwapHint:   "resembles core/x/y",
	}}
}

// TestCatalogFilesPlacement holds a Component and a Recipe with a file beside
// it to their places in the catalog tree, the hint above the Component.
func TestCatalogFilesPlacement(t *testing.T) {
	drafts := catalogTestDrafts()
	files := CatalogFiles(nil, drafts, catalogTestDecisions(tierHigh))
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	want := []string{
		"catalog/component/acme/resource/bucket/0.1.0/component.yaml",
		"catalog/recipe/terraform/bucket-acme/0.1.0/recipe.yaml",
		"catalog/recipe/terraform/bucket-acme/0.1.0/module/main.tf.json",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	if got, want := files[0].Content, "# resembles core/x/y\n"+drafts[0].Entity.Body; got != want {
		t.Errorf("component content =\n%s\nwant\n%s", got, want)
	}
	if files[1].Content != drafts[1].Entity.Body {
		t.Errorf("recipe content differs from its body")
	}
	if files[2].Content != "{\"resource\":{}}\n" {
		t.Errorf("file content = %q", files[2].Content)
	}
}

// TestCatalogFilesHints holds a hint to the tiers and settings that write it.
func TestCatalogFilesHints(t *testing.T) {
	body := catalogTestDrafts()[0].Entity.Body
	for _, c := range []struct {
		name     string
		tier     string
		settings string
		hinted   bool
	}{
		{"high", tierHigh, "", true},
		{"medium", tierMedium, "", true},
		{"low", tierLow, "", false},
		{"suppressed", tierHigh, `{"emit_hints":false}`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			files := CatalogFiles(json.RawMessage(c.settings), catalogTestDrafts()[:1], catalogTestDecisions(c.tier))
			want := body
			if c.hinted {
				want = "# resembles core/x/y\n" + body
			}
			if len(files) != 1 || files[0].Content != want {
				t.Errorf("files = %+v", files)
			}
		})
	}
}

// TestCatalogFilesLLMMode holds render_mode llm to the decision record alone.
func TestCatalogFilesLLMMode(t *testing.T) {
	files := CatalogFiles(json.RawMessage(`{"render_mode":"llm"}`), catalogTestDrafts(), catalogTestDecisions(tierHigh))
	if len(files) != 1 || files[0].Path != "import-decisions.json" {
		t.Fatalf("files = %+v", files)
	}
	if !strings.Contains(files[0].Content, `"swapHint":"resembles core/x/y"`) && !strings.Contains(files[0].Content, "resembles core/x/y") {
		t.Errorf("content = %s", files[0].Content)
	}
}

// TestCatalogFilesDeterministic holds two placements of one input to equal files.
func TestCatalogFilesDeterministic(t *testing.T) {
	a := CatalogFiles(nil, catalogTestDrafts(), catalogTestDecisions(tierHigh))
	b := CatalogFiles(nil, catalogTestDrafts(), catalogTestDecisions(tierHigh))
	if !reflect.DeepEqual(a, b) {
		t.Errorf("files differ:\n%v\n%v", a, b)
	}
}

// TestCatalogFilesJoinsFilePathsAsGiven holds a Recipe file's path to the
// version directory joined with it unchanged.
func TestCatalogFilesJoinsFilePathsAsGiven(t *testing.T) {
	d := DraftRecipe(RecipeDraft{Name: "r", Version: "1.0.0", Capability: "terraform", Files: []RecipeFile{{Path: "../x", Content: "c"}}})
	files := CatalogFiles(nil, []CatalogDraft{d}, nil)
	if got, want := files[1].Path, "catalog/recipe/terraform/r/1.0.0/../x"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

// TestCatalogFilesSkipsNonDrafts holds a referenced entity to no file.
func TestCatalogFilesSkipsNonDrafts(t *testing.T) {
	d := CatalogDraft{Entity: aggrega.EmittedEntity{Kind: "Component", Name: "a/b/c", Mode: "reference"}, Version: "1.0.0"}
	if files := CatalogFiles(nil, []CatalogDraft{d}, nil); len(files) != 0 {
		t.Errorf("files = %+v", files)
	}
}
