// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import "github.com/aggrega-ai/aggrega-pdk-go/pkg/importer"

// langflow is the one format this importer serves: Langflow flow JSON, parsed by
// the host as JSON and walked by walkTree.
var langflow = importer.Format{
	Name:     "langflow",
	Syntax:   "json",
	Mappings: mappings,
	Walk:     walkTree,
}

// mappings is the Langflow mapping arm: the standard-library component each
// Langflow node type maps to. The whole-flow wrapper component
// core/ai-agent/langflow is not a row here — it is one of the candidates the
// scorer weighs every node against, the low-confidence fallback node-level
// decomposition falls back to.
var mappings = map[string]importer.Mapping{
	"OpenAIModel":      {Component: "core/ai-model/openai", Category: "ai-model", Traits: map[string]string{"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"}, Vendor: "openai"},
	"AnthropicModel":   {Component: "core/ai-model/anthropic", Category: "ai-model", Traits: map[string]string{"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"}, Vendor: "anthropic"},
	"OpenAIEmbeddings": {Component: "core/ai-model/openai-embedding", Category: "ai-model", Traits: map[string]string{"model.aggrega.ai/capability": "embedding", "component.aggrega.ai/hosting": "managed"}, Vendor: "openai"},
	"Retrieval":        {Component: "core/ai-tool/retrieval", Category: "ai-tool", Traits: map[string]string{"tool.aggrega.ai/impact": "read-only"}, Vendor: "retrieval"},
	"APIRequest":       {Component: "core/ai-tool/http-request", Category: "ai-tool", Traits: map[string]string{"tool.aggrega.ai/impact": "mutating"}, Vendor: "http"},
}
