// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

// This file holds the knowledge the scorer weighs a node against: the candidate
// components and the mapping lookup. It lives inside the importer because
// domain-specific intelligence is the importer's to hold — the host mediates the
// pipeline and stays blind to what a node "means". An importer carries no live
// catalog access: it receives the source bytes and can ask the host to parse
// them, nothing more. So the knowledge the scorer needs — which component a node
// type maps to, and each candidate component's category, domain labels, and field
// shape — is carried here rather than read from a catalog at run time. Operators
// extend or override a format's mapping arm through the import attachment's
// settings (mapping_overrides); the arm is the default the overrides layer over.

// candidate is the metadata one candidate component exposes to the scorer: enough
// of the component's identity and shape for the five signals to weigh a source
// node against it without reading a live catalog. The fields mirror the metadata a
// real catalog component carries — its name, its ontology type and category
// labels, its refining domain labels, and the field names of its value, secret,
// and provided-output surfaces.
type candidate struct {
	name         string            // the component's fully-qualified name, e.g. core/ai-model/openai
	ontologyType string            // the reserved component.aggrega.ai/type ontology value
	category     string            // the component.aggrega.ai/category bucket
	domainLabels map[string]string // refining domain labels, e.g. model.aggrega.ai/capability=generative
	values       []string          // names of the component's declared value fields
	secrets      []string          // names of the component's declared secret fields
	provides     []string          // names of the outputs the component provides
}

// standardCandidates is the fixed set of candidate components the scorer weighs a
// node against, modelled on the standard-library ai-model, ai-tool, and ai-agent
// components. It holds the whole-flow wrapper components core/ai-agent/langflow
// and core/ai-agent/flowise: coarse-grained candidates that model an entire flow
// as one agent component, which a node-level decision surfaces as low-confidence
// runners-up. Every importer weighs every node against the whole set, whatever
// format it serves.
//
// The slice order is fixed so scoring iterates deterministically; the scorer
// re-sorts candidates by score and breaks ties by name, so the slice order never
// leaks into the output.
var standardCandidates = []candidate{
	{
		name:         "core/ai-model/openai",
		ontologyType: "resource",
		category:     "ai-model",
		domainLabels: map[string]string{"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"},
		values:       []string{"model", "temperature", "strict_response_format"},
	},
	{
		name:         "core/ai-model/anthropic",
		ontologyType: "resource",
		category:     "ai-model",
		domainLabels: map[string]string{"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"},
		values:       []string{"model", "temperature"},
	},
	{
		name:         "core/ai-model/openai-embedding",
		ontologyType: "resource",
		category:     "ai-model",
		domainLabels: map[string]string{"model.aggrega.ai/capability": "embedding", "component.aggrega.ai/hosting": "managed"},
		values:       []string{"model"},
	},
	{
		name:         "core/ai-tool/retrieval",
		ontologyType: "function",
		category:     "ai-tool",
		domainLabels: map[string]string{"tool.aggrega.ai/impact": "read-only"},
		values:       []string{"top_k", "side_effects"},
	},
	{
		name:         "core/ai-tool/http-request",
		ontologyType: "function",
		category:     "ai-tool",
		domainLabels: map[string]string{"tool.aggrega.ai/impact": "mutating"},
		values:       []string{"method", "payload_schema_ref"},
	},
	{
		name:         "core/ai-agent/langflow",
		ontologyType: "server",
		category:     "ai-agent",
		domainLabels: map[string]string{"agent.aggrega.ai/autonomy": "copilot"},
		values:       []string{"flow_id", "tweaks"},
	},
	{
		name:         "core/ai-agent/flowise",
		ontologyType: "server",
		category:     "ai-agent",
		domainLabels: map[string]string{"agent.aggrega.ai/autonomy": "copilot"},
		values:       []string{"chatflow_id", "override_config"},
	},
}

// mappingTable answers which mapping a node type has for one import: the
// operator's overrides for the requested format first (override precedence), and
// the served format's own arm second. The overrides layer over the arm so an
// operator can add a node type the arm does not cover or redirect one it does,
// without rebuilding the importer. The arm is consulted only when the request
// names the format the importer serves.
type mappingTable struct {
	overrides mappingOverrides
	format    string
	arm       map[string]Mapping
}

// newMappingTable builds the lookup for one import of format by an importer
// serving the format served describes.
func newMappingTable(overrides mappingOverrides, format string, served Format) mappingTable {
	t := mappingTable{overrides: overrides, format: format}
	if format == served.Name {
		t.arm = served.Mappings
	}
	return t
}

// lookup returns the mapping for a node type, or false when neither the
// overrides nor the arm map it.
func (t mappingTable) lookup(nodeType string) (Mapping, bool) {
	if arm, ok := t.overrides[t.format]; ok {
		if e, ok := arm[nodeType]; ok {
			return e, true
		}
	}
	if e, ok := t.arm[nodeType]; ok {
		return e, true
	}
	return Mapping{}, false
}
