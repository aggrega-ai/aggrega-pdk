// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

// This file holds the importer's embedded domain knowledge: the node-type
// mapping table and the component metadata the confidence scorer weighs a source
// node against. It lives inside the guest because domain-specific intelligence is
// the guest's to hold — the host mediates the pipeline and stays blind to what a
// node "means". The guest carries no live catalog access: it receives the source
// bytes and can ask the host to parse them, nothing more. So the knowledge the
// scorer needs — which component a node-type maps to, and each candidate
// component's category, domain labels, and field shape — is embedded here rather
// than read from a catalog at run time. Operators extend or override the mapping
// table through the import attachment's settings (see mapping_overrides in
// settings.go); the embedded table is the default the overrides layer over.

// componentKnowledge is the embedded metadata one candidate component exposes to
// the scorer: enough of the component's identity and shape for the five signals
// to weigh a source node against it without reading a live catalog. The fields
// mirror the metadata a real catalog component carries — its name, its ontology
// type and category labels, its refining domain labels, and the field names of
// its value, secret, and provided-output surfaces.
type componentKnowledge struct {
	name         string            // the component's fully-qualified name, e.g. core/ai-model/openai
	ontologyType string            // the reserved component.aggrega.ai/type ontology value
	category     string            // the component.aggrega.ai/category bucket
	domainLabels map[string]string // refining domain labels, e.g. model.aggrega.ai/capability=generative
	values       []string          // names of the component's declared value fields
	secrets      []string          // names of the component's declared secret fields
	provides     []string          // names of the outputs the component provides
}

// embeddedCatalog is the fixed set of candidate components the scorer weighs a
// node against, modelled on the standard-library ai-model, ai-tool, and ai-agent
// components. The slice order is fixed so scoring iterates deterministically; the
// scorer re-sorts candidates by score and breaks ties by name, so the slice order
// never leaks into the output.
var embeddedCatalog = []componentKnowledge{
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

// componentByName returns the embedded knowledge for a component name, or nil
// when the name is not one the importer knows.
func componentByName(name string) *componentKnowledge {
	for i := range embeddedCatalog {
		if embeddedCatalog[i].name == name {
			return &embeddedCatalog[i]
		}
	}
	return nil
}

// tableEntry is one node-type mapping: the component the node-type maps to, the
// category the node belongs to, the refining traits the node-type implies (scored
// against a candidate's domain labels), and the vendor token the node-type carries
// (scored against a candidate's name). A mapping is the strongest signal the
// scorer weighs — a human-authored statement that a node-type IS a component.
type tableEntry struct {
	component string
	category  string
	traits    map[string]string
	vendor    string
}

// mappingTable is the embedded (format → node-type → mapping) table. Each format
// is an independent arm, so a new source format is added by appending an arm here
// with no change elsewhere. v1 ships the langflow and flowise arms; the whole-flow
// wrapper components (core/ai-agent/{langflow,flowise}) are the low-confidence
// fallback candidates node-level decomposition falls back to, not table entries.
var mappingTable = map[string]map[string]tableEntry{
	"langflow": {
		"OpenAIModel":      {component: "core/ai-model/openai", category: "ai-model", traits: map[string]string{"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"}, vendor: "openai"},
		"AnthropicModel":   {component: "core/ai-model/anthropic", category: "ai-model", traits: map[string]string{"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"}, vendor: "anthropic"},
		"OpenAIEmbeddings": {component: "core/ai-model/openai-embedding", category: "ai-model", traits: map[string]string{"model.aggrega.ai/capability": "embedding", "component.aggrega.ai/hosting": "managed"}, vendor: "openai"},
		"Retrieval":        {component: "core/ai-tool/retrieval", category: "ai-tool", traits: map[string]string{"tool.aggrega.ai/impact": "read-only"}, vendor: "retrieval"},
		"APIRequest":       {component: "core/ai-tool/http-request", category: "ai-tool", traits: map[string]string{"tool.aggrega.ai/impact": "mutating"}, vendor: "http"},
	},
	"flowise": {
		"chatOpenAI":       {component: "core/ai-model/openai", category: "ai-model", traits: map[string]string{"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"}, vendor: "openai"},
		"chatAnthropic":    {component: "core/ai-model/anthropic", category: "ai-model", traits: map[string]string{"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"}, vendor: "anthropic"},
		"openAIEmbeddings": {component: "core/ai-model/openai-embedding", category: "ai-model", traits: map[string]string{"model.aggrega.ai/capability": "embedding", "component.aggrega.ai/hosting": "managed"}, vendor: "openai"},
		"retrieverTool":    {component: "core/ai-tool/retrieval", category: "ai-tool", traits: map[string]string{"tool.aggrega.ai/impact": "read-only"}, vendor: "retrieval"},
		"httpRequest":      {component: "core/ai-tool/http-request", category: "ai-tool", traits: map[string]string{"tool.aggrega.ai/impact": "mutating"}, vendor: "http"},
	},
}

// lookupMapping returns the mapping for a (format, node-type), consulting the
// operator overrides first (override precedence) and the embedded table second.
// The overrides layer over the embedded defaults so an operator can add a
// node-type the embedded table does not cover or redirect one it does, without
// rebuilding the guest.
func lookupMapping(overrides mappingOverrides, format, nodeType string) (tableEntry, bool) {
	if arm, ok := overrides[format]; ok {
		if e, ok := arm[nodeType]; ok {
			return e, true
		}
	}
	if arm, ok := mappingTable[format]; ok {
		if e, ok := arm[nodeType]; ok {
			return e, true
		}
	}
	return tableEntry{}, false
}

// wrapperComponentFor returns the whole-flow wrapper component for a source
// format — the coarse-grained fallback candidate that models the entire flow as
// one agent component. It is a low-confidence candidate the decision record
// surfaces as a runner-up, never the node-level primary.
func wrapperComponentFor(format string) string {
	switch format {
	case "langflow":
		return "core/ai-agent/langflow"
	case "flowise":
		return "core/ai-agent/flowise"
	default:
		return ""
	}
}
