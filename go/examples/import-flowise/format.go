// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import (
	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
	"github.com/aggrega-ai/aggrega-pdk-go/pkg/importer"
)

// flowise is the one format this importer serves: Flowise flow JSON, parsed by
// the host as JSON and walked by walkTree.
var flowise = importer.Format{
	Name:     "flowise",
	Syntax:   "json",
	Mappings: mappings,
	Walk:     walkTree,
}

// detect is how a Flowise flow is recognised when an import names no format,
// declared in describe() and recorded on the importer's Plugin entity. A Flowise
// export — a chatflow or an agentflow — keeps its graph flat at the top level,
// so a source that parses as JSON with arrays at nodes and edges is one. The
// third predicate asks for nothing at data.nodes, where a flow that nests its
// graph carries it, so a source carrying both shapes is not claimed by this
// signature as well. The host evaluates the signature without knowing what
// Flowise is.
var detect = []aggrega.DetectSignature{{
	Syntax: flowise.Syntax,
	Match: []aggrega.PathMatch{
		{Path: "nodes", Is: "array"},
		{Path: "edges", Is: "array"},
		{Path: "data.nodes", Is: "absent"},
	},
}}

// mappings is the Flowise mapping arm: the standard-library component each
// Flowise node type maps to. A row draws its signal only where the workspace's
// catalog holds the component it names. The whole-flow wrapper component
// core/ai-agent/flowise is not a row here: where the catalog holds it, the
// scorer weighs every node against it as it does every other catalog Component.
var mappings = map[string]importer.Mapping{
	"chatOpenAI":       {Component: "core/ai-model/openai", Category: "ai-model", Traits: map[string]string{"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"}, Vendor: "openai"},
	"chatAnthropic":    {Component: "core/ai-model/anthropic", Category: "ai-model", Traits: map[string]string{"model.aggrega.ai/capability": "generative", "component.aggrega.ai/hosting": "managed"}, Vendor: "anthropic"},
	"openAIEmbeddings": {Component: "core/ai-model/openai-embedding", Category: "ai-model", Traits: map[string]string{"model.aggrega.ai/capability": "embedding", "component.aggrega.ai/hosting": "managed"}, Vendor: "openai"},
	"retrieverTool":    {Component: "core/ai-tool/retrieval", Category: "ai-tool", Traits: map[string]string{"tool.aggrega.ai/impact": "read-only"}, Vendor: "retrieval"},
	"httpRequest":      {Component: "core/ai-tool/http-request", Category: "ai-tool", Traits: map[string]string{"tool.aggrega.ai/impact": "mutating"}, Vendor: "http"},
}
