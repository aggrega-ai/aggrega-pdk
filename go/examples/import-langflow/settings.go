// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import "encoding/json"

// importSettings is the guest's interpretation of the opaque settings document
// the consumer supplies. The host pipes the document through without reading it;
// the meaning of every field is the guest's. Absent or malformed settings leave
// every field at its default, so a settings-free run is well-defined and
// byte-identical to one carrying an empty document.
type importSettings struct {
	// TargetNamespace is the namespace segment drafted entities are named under —
	// the <ns> in <ns>/<category>/<name>. It never defaults to a protected
	// namespace; drafts belong to the author, so an unset value falls back to the
	// neutral "imported" segment rather than core or example.
	TargetNamespace string `json:"target_namespace"`
	// ReuseMode governs whether a high-confidence match is referenced or only
	// hinted. The default hints-only references nothing silently — every node is
	// drafted, with strong matches recorded as swap hints. auto-apply-above-threshold
	// references a match scoring at or above ReuseThreshold.
	ReuseMode string `json:"reuse_mode"`
	// ReuseThreshold is the score at or above which auto-apply references a
	// component. It is consulted only in auto-apply-above-threshold mode.
	ReuseThreshold float64 `json:"reuse_threshold"`
	// EmitHints governs whether resemblance hints are written as inline comments
	// in the drafted files. The structured hints in the decision record are always
	// emitted; this only suppresses the inline comment noise.
	EmitHints *bool `json:"emit_hints"`
	// RenderMode selects which renderer turns the shared decision record into
	// files: human (the default) chunks it into per-entity drafts with inline
	// hints; llm serves the whole decision record as one document for a richer
	// downstream narrative. Both renderers consume the identical decision record.
	RenderMode string `json:"render_mode"`
	// MappingOverrides extends or overrides the embedded node-type mapping table,
	// keyed by format then node-type, with override precedence over the embedded
	// defaults.
	MappingOverrides mappingOverrides `json:"mapping_overrides"`
}

// mappingOverrides is the operator-supplied extension of the embedded mapping
// table: format → node-type → mapping, layered over the embedded defaults with
// override precedence.
type mappingOverrides map[string]map[string]tableEntry

// UnmarshalJSON decodes a mapping-overrides document, whose entries carry the
// same shape as an embedded table entry but with lowercase JSON keys.
func (m *mappingOverrides) UnmarshalJSON(data []byte) error {
	var raw map[string]map[string]struct {
		Component string            `json:"component"`
		Category  string            `json:"category"`
		Traits    map[string]string `json:"traits"`
		Vendor    string            `json:"vendor"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(mappingOverrides, len(raw))
	for format, arm := range raw {
		entries := make(map[string]tableEntry, len(arm))
		for nodeType, e := range arm {
			entries[nodeType] = tableEntry{component: e.Component, category: e.Category, traits: e.Traits, vendor: e.Vendor}
		}
		out[format] = entries
	}
	*m = out
	return nil
}

const (
	reuseModeHintsOnly = "hints-only"
	reuseModeAutoApply = "auto-apply-above-threshold"
	renderModeHuman    = "human"
	renderModeLLM      = "llm"
	defaultNamespace   = "imported"
	defaultThreshold   = 0.85
)

// parseSettings decodes the opaque settings document into a typed, defaulted
// settings value. A nil or malformed document yields the defaults rather than an
// error, so an importer with no settings behaves identically to one whose
// document decoded cleanly to the defaults.
func parseSettings(raw json.RawMessage) importSettings {
	s := importSettings{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &s)
	}
	if s.TargetNamespace == "" {
		s.TargetNamespace = defaultNamespace
	}
	if s.ReuseMode == "" {
		s.ReuseMode = reuseModeHintsOnly
	}
	if s.ReuseThreshold == 0 {
		s.ReuseThreshold = defaultThreshold
	}
	if s.RenderMode == "" {
		s.RenderMode = renderModeHuman
	}
	return s
}

// emitInlineHints reports whether inline resemblance comments are written, which
// defaults to true when the setting is unset.
func (s importSettings) emitInlineHints() bool {
	return s.EmitHints == nil || *s.EmitHints
}
