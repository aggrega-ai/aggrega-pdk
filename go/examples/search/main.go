// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Example search plugin for the Aggrega Wasm runtime.
//
// It answers a ranked full-text query over a small inverted index the guest
// embeds at build time. describe() self-reports plugin_type "search", declares
// the search export, and stamps the index format and version the guest reads;
// the search export tokenises the query, scores each document by how often the
// query terms occur in it, and returns the matches in descending score order
// with a document-id tie-break so identical input yields identical output.
//
// The index the guest embeds is index-only by design: per document it carries
// the tokens the query is matched against plus the display fields a result
// needs — the title, an in-document anchor, and a short snippet. It does NOT
// carry full page bodies; a consumer that wants a matched document's body
// fetches it from the content bundle by the returned document id, keeping the
// guest small and its query surface offline.
//
// The host verifies end-to-end that:
//
//   - The plugin loaded through the capability-denied build profile.
//   - describe() reported a valid search descriptor with the search export.
//   - search read the SearchInput query and returned ranked SearchResults.
//   - Identical input produced byte-identical, identically-ordered output.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o search-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// pluginDescriptor is the static describe() self-report: a v1alpha1 search
// plugin whose search export the host validates before dispatch. The
// description names the index format and version the guest is built against so
// a reader of describe() sees the exact index contract the guest reads; the
// search export declares no host functions, so the guest reaches no host data
// surface and answers every query from its embedded index alone.
const pluginDescriptor = `{"abi_version":"v1alpha1","plugin_type":"search","description":"Ranked full-text search over an embedded inverted index (format aggrega.search.index/v1).","exports":[{"name":"search","description":"Answer a ranked full-text query over the embedded index.","stability":"stable"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}`

// prefixFilterKey is the one field filter this fixture interprets: a value
// under this key restricts the result set to documents whose id begins with
// the value. The search socket leaves the filter vocabulary to the guest; a
// filter under any other key matches nothing here and is otherwise ignored.
const prefixFilterKey = "prefix"

// indexedDoc is one document in the embedded index. Terms are the tokens the
// query is matched against — the index — separate from Title, Anchor, and
// Snippet, which are the display fields a result carries. No full page body is
// embedded: the document id is the handle a consumer resolves the body by.
type indexedDoc struct {
	ID      string
	Title   string
	Anchor  string
	Snippet string
	Terms   []string
}

// corpus is the fixture's embedded index. It is a handful of documents whose
// terms let a query like "compiler" or "compiler render" produce a stable
// ranked order, so an end-to-end test can assert an exact result sequence.
var corpus = []indexedDoc{
	{
		ID:      "architecture/invariants",
		Title:   "Architectural Invariants",
		Anchor:  "the-compiler-is-dumb",
		Snippet: "The compiler is a dumb, unopinionated text router.",
		Terms:   []string{"compiler", "invariant", "router", "text", "dumb", "aot"},
	},
	{
		ID:      "architecture/pipeline",
		Title:   "Compilation Pipeline",
		Anchor:  "phases",
		Snippet: "The compiler pipeline renders the graph in ordered phases.",
		Terms:   []string{"compiler", "pipeline", "phase", "render", "graph"},
	},
	{
		ID:      "language-reference/component",
		Title:   "Component Entity",
		Anchor:  "schema",
		Snippet: "A Component declares its data pillars under a schema.",
		Terms:   []string{"component", "schema", "values", "infra", "provides"},
	},
	{
		ID:      "language-reference/recipe",
		Title:   "Recipe Entity",
		Anchor:  "values-mapping",
		Snippet: "A Recipe maps values into provider render input.",
		Terms:   []string{"recipe", "values", "mapping", "provider", "render"},
	},
}

// postings is the inverted index built from the corpus at guest init: a term
// maps to the indices of the documents that carry it. It is derived, not
// embedded, so the index and the display fields stay single-sourced in corpus.
var postings = buildPostings()

func buildPostings() map[string][]int {
	p := make(map[string][]int)
	for i, d := range corpus {
		for _, term := range d.Terms {
			p[term] = append(p[term], i)
		}
	}
	return p
}

// tokenize lowercases s and splits it into alphanumeric tokens, discarding
// punctuation and empties. The query grammar is the guest's own; the host
// passes the query string through verbatim and does not parse it.
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	return fields
}

//go:wasmexport describe
func describe() int32 {
	pdk.Output([]byte(pluginDescriptor))
	return 0
}

//go:wasmexport search
func search() int32 {
	var in aggrega.SearchInput
	if err := json.Unmarshal(pdk.Input(), &in); err != nil {
		pdk.SetError(fmt.Errorf("search-go: unmarshal SearchInput: %w", err))
		return 1
	}

	prefix, hasPrefix := in.Filters[prefixFilterKey]

	// Score every document by the total occurrences of the query's tokens in
	// its terms. A document with no query-term match scores zero and is
	// dropped, so the result set is the matching documents alone.
	type scored struct {
		idx   int
		score float64
	}
	queryTokens := tokenize(in.Query)
	var hits []scored
	for i, d := range corpus {
		if hasPrefix && !strings.HasPrefix(d.ID, prefix) {
			continue
		}
		var score float64
		for _, qt := range queryTokens {
			for _, term := range d.Terms {
				if term == qt {
					score++
				}
			}
		}
		if score > 0 {
			hits = append(hits, scored{idx: i, score: score})
		}
	}

	// Rank by descending score, breaking ties on ascending document id so the
	// output order is a deterministic function of the input — identical input
	// yields identical order, the property a diff-based consumer relies on.
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].score != hits[b].score {
			return hits[a].score > hits[b].score
		}
		return corpus[hits[a].idx].ID < corpus[hits[b].idx].ID
	})

	// A positive limit caps the result set at that size; zero or absent returns
	// every match.
	if in.Limit > 0 && len(hits) > in.Limit {
		hits = hits[:in.Limit]
	}

	results := make([]aggrega.SearchResult, 0, len(hits))
	for _, h := range hits {
		d := corpus[h.idx]
		results = append(results, aggrega.SearchResult{
			DocID:   d.ID,
			Title:   d.Title,
			Anchor:  d.Anchor,
			Snippet: d.Snippet,
			Score:   h.score,
		})
	}

	out := aggrega.SearchOutput{Results: results}
	outputBytes, err := json.Marshal(out)
	if err != nil {
		pdk.SetError(fmt.Errorf("search-go: marshal SearchOutput: %w", err))
		return 1
	}
	pdk.Output(outputBytes)
	return 0
}

func main() {}
