// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Fixture search plugin for the Aggrega Wasm runtime.
 *
 * It answers a ranked full-text query over a small inverted index the
 * guest embeds at build time. describe() self-reports plugin_type
 * "search", declares the search export, and stamps the index format and
 * version the guest reads; the search export tokenises the query, scores
 * each document by how often the query terms occur in it, and returns the
 * matches in descending score order with a document-id tie-break so
 * identical input yields identical output.
 *
 * The index the guest embeds is index-only by design: per document it
 * carries the tokens the query is matched against plus the display fields a
 * result needs — the title, an in-document anchor, and a short snippet. It
 * does NOT carry full page bodies; a consumer that wants a matched
 * document's body fetches it from the content bundle by the returned
 * document id, keeping the guest small and its query surface offline. The
 * search export declares no host functions, so the guest reaches no host
 * data surface and answers every query from its embedded index alone.
 *
 * Entry convention follows the Extism JS PDK: parameterless function,
 * read input via Host.inputString(), write output via
 * Host.outputString(), expose via module.exports. The search codec is
 * consumed from the TypeScript Plugin Development Kit, which re-exports it
 * from the portable ABI package.
 */

import {
  decodeSearchInput,
  encodeSearchOutput,
  type SearchResult,
} from "@aggrega-ai/aggrega-pdk-typescript";

// The static describe() self-report: a v1alpha1 search plugin whose
// search export the host validates before dispatch. The description names
// the index format and version the guest is built against so a reader of
// describe() sees the exact index contract the guest reads.
const PLUGIN_DESCRIPTOR =
  '{"abi_version":"v1alpha1","plugin_type":"search","description":"Ranked full-text search over an embedded inverted index (format aggrega.search.index/v1).","exports":[{"name":"search","description":"Answer a ranked full-text query over the embedded index.","stability":"stable"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}';

// The one field filter this fixture interprets: a value under this key
// restricts the result set to documents whose id begins with the value.
// The search socket leaves the filter vocabulary to the guest; a filter
// under any other key matches nothing here and is otherwise ignored.
const PREFIX_FILTER_KEY = "prefix";

interface IndexedDoc {
  id: string;
  title: string;
  anchor: string;
  snippet: string;
  terms: string[];
}

// The fixture's embedded index. A handful of documents whose terms let a
// query like "compiler" or "compiler render" produce a stable ranked
// order, so an end-to-end test can assert an exact result sequence. Terms
// are the tokens the query is matched against — the index — separate from
// the title, anchor, and snippet, the display fields a result carries. No
// full page body is embedded: the id is the handle a consumer resolves the
// body by.
const CORPUS: IndexedDoc[] = [
  {
    id: "architecture/invariants",
    title: "Architectural Invariants",
    anchor: "the-compiler-is-dumb",
    snippet: "The compiler is a dumb, unopinionated text router.",
    terms: ["compiler", "invariant", "router", "text", "dumb", "aot"],
  },
  {
    id: "architecture/pipeline",
    title: "Compilation Pipeline",
    anchor: "phases",
    snippet: "The compiler pipeline renders the graph in ordered phases.",
    terms: ["compiler", "pipeline", "phase", "render", "graph"],
  },
  {
    id: "language-reference/component",
    title: "Component Entity",
    anchor: "schema",
    snippet: "A Component declares its data pillars under a schema.",
    terms: ["component", "schema", "values", "infra", "provides"],
  },
  {
    id: "language-reference/recipe",
    title: "Recipe Entity",
    anchor: "values-mapping",
    snippet: "A Recipe maps values into provider render input.",
    terms: ["recipe", "values", "mapping", "provider", "render"],
  },
];

// tokenize lowercases s and splits it into alphanumeric tokens, discarding
// punctuation and empties. The query grammar is the guest's own; the host
// passes the query string through verbatim and does not parse it.
function tokenize(s: string): string[] {
  const out: string[] = [];
  let cur = "";
  for (const ch of s.toLowerCase()) {
    if ((ch >= "a" && ch <= "z") || (ch >= "0" && ch <= "9")) {
      cur += ch;
    } else if (cur !== "") {
      out.push(cur);
      cur = "";
    }
  }
  if (cur !== "") {
    out.push(cur);
  }
  return out;
}

// The host calls describe() once at load to read the plugin's static
// self-report before dispatching search.
function describe(): void {
  Host.outputString(PLUGIN_DESCRIPTOR);
}

function search(): void {
  const input = decodeSearchInput(Host.inputString());
  const prefix = input.filters ? input.filters[PREFIX_FILTER_KEY] : undefined;
  const queryTokens = tokenize(input.query);

  // Score every document by the total occurrences of the query's tokens in
  // its terms. A document with no query-term match scores zero and is
  // dropped, so the result set is the matching documents alone.
  const hits: { idx: number; score: number }[] = [];
  for (let i = 0; i < CORPUS.length; i++) {
    const doc = CORPUS[i];
    if (prefix !== undefined && !doc.id.startsWith(prefix)) {
      continue;
    }
    let score = 0;
    for (const qt of queryTokens) {
      for (const term of doc.terms) {
        if (term === qt) {
          score++;
        }
      }
    }
    if (score > 0) {
      hits.push({ idx: i, score });
    }
  }

  // Rank by descending score, breaking ties on ascending document id so the
  // output order is a deterministic function of the input — identical input
  // yields identical order, the property a diff-based consumer relies on.
  hits.sort((a, b) => {
    if (a.score !== b.score) {
      return b.score - a.score;
    }
    const ida = CORPUS[a.idx].id;
    const idb = CORPUS[b.idx].id;
    return ida < idb ? -1 : ida > idb ? 1 : 0;
  });

  // A positive limit caps the result set at that size; zero or absent
  // returns every match.
  const limit = input.limit ?? 0;
  const capped = limit > 0 && hits.length > limit ? hits.slice(0, limit) : hits;

  const results: SearchResult[] = capped.map((h) => {
    const doc = CORPUS[h.idx];
    return {
      doc_id: doc.id,
      title: doc.title,
      anchor: doc.anchor,
      snippet: doc.snippet,
      score: h.score,
    };
  });

  Host.outputString(encodeSearchOutput({ results }));
}

module.exports = { search, describe };
