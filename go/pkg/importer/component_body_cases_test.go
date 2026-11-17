// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

// componentBodyCases are the nodes whose drafted Component body is held in
// testdata/component-body, one file per case.
var componentBodyCases = []struct {
	file string
	name string
	node Node
}{
	{"empty.yaml", "acme/ai-tool/plain", Node{}},
	{"fields.yaml", "acme/ai-model/chat", Node{Fields: []string{"model", "temperature"}}},
	{"secrets.yaml", "acme/ai-agent/runner", Node{Secrets: []string{"api_key", "token"}}},
	{"both.yaml", "acme/ai-tool/search", Node{Fields: []string{"endpoint", "limit"}, Secrets: []string{"api_key"}}},
	{"category-component.yaml", "acme/component/thing", Node{Fields: []string{"size"}, Secrets: []string{"password"}}},
	{"field-type.yaml", "acme/ai-model/typed", Node{Fields: []string{"type", "name"}}},
}
