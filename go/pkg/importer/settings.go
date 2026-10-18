// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import "encoding/json"

// importSettings is the importer's interpretation of the opaque settings document
// the consumer supplies. The host pipes the document through without reading it;
// the meaning of every field is the importer's. An absent document, or one that is
// not valid JSON, leaves every field at its default, so a settings-free run is
// well-defined and byte-identical to one carrying an empty document.
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
	// EmptyUnmarkedCredentials governs whether the flow a platform carries has
	// written empty the inputs its format marks as no credential and that can
	// carry one all the same — request headers, query parameters, a request
	// written as a command line, an input named for a key. False, the default,
	// carries each as written, a value that is not a JSON boolean reading as
	// false. What a format marks as a credential is written empty whatever this
	// says, and so is what a format writes empty because it belongs to a part of
	// the flow — an MCP server's members, or what a node the platform's
	// connection reaches would send in place of what that connection supplies;
	// [Platform.Credentials] is handed this and tells the two apart.
	EmptyUnmarkedCredentials bool `json:"empty_unmarked_credentials"`
	// RenderMode selects which renderer turns the shared decision record into
	// files: human (the default) chunks it into per-entity drafts with inline
	// hints; llm serves the whole decision record as one document for a richer
	// downstream narrative. Both renderers consume the identical decision record.
	RenderMode string `json:"render_mode"`
	// MappingOverrides extends or overrides the served format's mapping arm,
	// keyed by format then node type, with override precedence over the arm.
	MappingOverrides mappingOverrides `json:"mapping_overrides"`
	// Layout selects how a source of a format whose flows run on a platform is
	// drafted. platform, the default, drafts one instance of the platform's
	// Component carrying the source in its definition value, with each part of the
	// flow a socket of the platform admits drafted as a Component wired to that
	// socket; nodes drafts every node of the source as a Component of its own. A
	// format that describes no platform is drafted node by node whatever this
	// says.
	Layout string `json:"layout"`
	// AttachEgressAllowlist governs whether each platform instance the platform
	// draft writes carries core/egress-allowlist, the Feature that lets a
	// component connect only to what it is wired to, attached under the key
	// egress-allowlist. The draft never sets the Feature's infrastructure: an
	// Aggregate's attachment carries none, so its comment names, for the Release,
	// the affirmation the Feature's realization halts without and the address
	// ranges a target the cluster does not run is reached at. False, the default,
	// attaches nothing, a value that is not a JSON boolean reading as false. A
	// draft that writes no platform — the node-by-node draft, or a format that
	// describes none — attaches nothing whatever this says, and where it is true
	// the import warns that nothing was attached.
	AttachEgressAllowlist bool `json:"attach_egress_allowlist"`
}

// mappingOverrides is the operator-supplied extension of a mapping arm:
// format → node type → mapping, layered over the arm with override precedence.
type mappingOverrides map[string]map[string]Mapping

// UnmarshalJSON decodes a mapping-overrides document, whose entries carry the
// same shape as a [Mapping] with lowercase JSON keys.
//
// The document is decoded here rather than through tags on [Mapping], and that is
// load-bearing: an error this decoder returns ends the decode of the whole settings
// document, where a mismatched field under a tag-only decode would be skipped and
// the decode carried on, so the two would leave different settings behind for one
// malformed overrides block.
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
		entries := make(map[string]Mapping, len(arm))
		for nodeType, e := range arm {
			entries[nodeType] = Mapping{Component: e.Component, Category: e.Category, Traits: e.Traits, Vendor: e.Vendor}
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
	layoutPlatform     = "platform"
	layoutNodes        = "nodes"
	defaultNamespace   = "imported"
	defaultThreshold   = 0.85
)

// parseSettings decodes the opaque settings document into a typed, defaulted
// settings value. A nil document, or one that is not valid JSON, yields the
// defaults rather than an error, so an importer with no settings behaves
// identically to one whose document decoded cleanly to the defaults.
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
	if s.Layout == "" {
		s.Layout = layoutPlatform
	}
	return s
}

// nodeLayout reports whether the settings ask for the node-by-node draft. Any
// other value, the default included, asks for the platform draft; a value that
// is neither layout is reported where the platform draft is made.
func (s importSettings) nodeLayout() bool {
	return s.Layout == layoutNodes
}

// egressAllowlist is the Feature the attach_egress_allowlist setting attaches
// to a drafted platform, and egressAllowlistKey the key it is attached under:
// the Feature's own name, less the namespace an attachment's key cannot carry.
// egressAffirmation and egressRanges are the two members of the Feature's
// infrastructure a draft names for the Release and never sets.
const (
	egressAllowlist    = "core/egress-allowlist"
	egressAllowlistKey = "egress-allowlist"
	egressAffirmation  = "infra.control_plane_present"
	egressRanges       = "infra.admitted_destinations"
)

// emitInlineHints reports whether inline resemblance comments are written, which
// defaults to true when the setting is unset.
func (s importSettings) emitInlineHints() bool {
	return s.EmitHints == nil || *s.EmitHints
}
