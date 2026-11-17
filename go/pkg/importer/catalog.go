// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"encoding/json"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// CatalogFiles lays drafted catalog entities out at the standard-library tree
// under catalog/, under the render_mode and emit_hints settings the settings
// document holds, as [Render] reads them: a Component at
// catalog/component/<Name>/<Version>/component.yaml, a Recipe at
// catalog/recipe/<Capability>/<Name>/<Version>/recipe.yaml with each of its
// Files beside it, a HIGH or MEDIUM match's hint written above a drafted
// Component, in the order the drafts are given; with render_mode llm, the
// decisions alone as one JSON document. A draft that is not a drafted Component
// or Recipe writes no file.
//
// A Recipe file's path is joined to its version directory as given: the host
// already refuses a drafted path that leaves the output directory, so this
// writer passes no second judgement on one.
func CatalogFiles(settings json.RawMessage, drafts []CatalogDraft, decisions []aggrega.ImportDecision) []aggrega.ScaffoldFile {
	s := parseSettings(settings)
	if s.RenderMode == renderModeLLM {
		return renderLLM(decisions)
	}
	byDecisionRef := decisionsByRef(decisions)

	var files []aggrega.ScaffoldFile
	for _, d := range drafts {
		e := d.Entity
		if e.Mode != "draft" {
			continue
		}
		switch e.Kind {
		case "Component":
			files = append(files, aggrega.ScaffoldFile{
				Path:    "catalog/component/" + e.Name + "/" + d.Version + "/component.yaml",
				Content: withInlineHint(s, byDecisionRef, e),
			})
		case "Recipe":
			dir := "catalog/recipe/" + d.Capability + "/" + e.Name + "/" + d.Version + "/"
			files = append(files, aggrega.ScaffoldFile{Path: dir + "recipe.yaml", Content: e.Body})
			for _, f := range d.Files {
				files = append(files, aggrega.ScaffoldFile{Path: dir + f.Path, Content: f.Content})
			}
		}
	}
	return files
}
