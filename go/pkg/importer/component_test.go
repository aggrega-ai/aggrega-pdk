// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDraftComponentBodyUnchanged holds the body drafted for a source node to
// the files captured in testdata/component-body.
func TestDraftComponentBodyUnchanged(t *testing.T) {
	for _, c := range componentBodyCases {
		t.Run(c.file, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("testdata", "component-body", c.file))
			if err != nil {
				t.Fatal(err)
			}
			if got := draftComponentBody(c.name, c.node); got != string(want) {
				t.Errorf("draftComponentBody(%q) =\n%s\nwant\n%s", c.name, got, want)
			}
		})
	}
}

func goldenComponentDraft() ComponentDraft {
	maxLen := 63
	one := json.Number("1")
	return ComponentDraft{
		Name:        "acme/resource/bucket",
		Version:     "0.1.0",
		Description: "A storage bucket.",
		Labels: []Label{
			{Key: "component.aggrega.ai/type", Value: "resource"},
			{Key: "component.aggrega.ai/category", Value: "storage"},
		},
		Values: []SchemaNode{
			{Name: "name", Type: []string{"string"}, Description: "[vendor schema text] The bucket name.", Required: true, MaxLength: &maxLen, Pattern: "^[a-z0-9-]+$"},
			{Name: "versioning", Type: []string{"boolean"}},
		},
		Infra: []SchemaNode{
			{Name: "overrides", Type: []string{"object"}, Open: true, Comment: []string{"free-form {{ settings"}},
			{Name: "replicas", Type: []string{"integer", "string"}, Min: &one},
		},
		Provides: []SchemaNode{
			{Name: "arn", Type: []string{"string"}, Sensitive: true},
		},
		Slots: []ComponentSlot{
			{Name: "credential", Selector: "feature.aggrega.ai/kind=secret", Sensitive: true, Required: true, Description: "Credential slot."},
			{Name: "network", Selector: "feature.aggrega.ai/kind=network", Inject: []string{"env"}},
		},
		Inject: []string{"env", "file"},
	}
}

func one2(n int) []int { return []int{n} }

// TestDraftComponentGolden holds a draft carrying every pillar, a secret slot
// and an open object to the file in testdata/component.
func TestDraftComponentGolden(t *testing.T) {
	got := DraftComponent(goldenComponentDraft())
	want, err := os.ReadFile(filepath.Join("testdata", "component", "golden.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Entity.Body != string(want) {
		t.Errorf("DraftComponent body =\n%s\nwant\n%s", got.Entity.Body, want)
	}
	if got.Entity.Kind != "Component" || got.Entity.Mode != "draft" || got.Entity.Name != "acme/resource/bucket" {
		t.Errorf("entity = %+v", got.Entity)
	}
	if got.Version != "0.1.0" {
		t.Errorf("Version = %q", got.Version)
	}
}

// TestDraftComponentIsDeterministic holds two drafts of one input to equal bytes.
func TestDraftComponentIsDeterministic(t *testing.T) {
	a := DraftComponent(goldenComponentDraft())
	b := DraftComponent(goldenComponentDraft())
	if a.Entity.Body != b.Entity.Body {
		t.Errorf("bodies differ:\n%s\n%s", a.Entity.Body, b.Entity.Body)
	}
}

// TestDraftComponentWritesNoPillarForNil holds a draft without pillars or
// slots to an empty spec.
func TestDraftComponentWritesNoPillarForNil(t *testing.T) {
	got := DraftComponent(ComponentDraft{Name: "a/b/c", Version: "1.0.0"}).Entity.Body
	want := "apiVersion: aggrega.ai/v1beta1\nkind: Component\nmetadata:\n  name: a/b/c\n  version: 1.0.0\nspec: {}\n"
	if got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"AWS::S3::Bucket": "aws-s3-bucket", "  a  b ": "a-b", "": ""} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEditDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"kitten", "sitting", 3}, {"", "abc", 3}, {"same", "same", 0}, {"é", "e", 1}} {
		if got := EditDistance(c.a, c.b); got != c.want {
			t.Errorf("EditDistance(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
