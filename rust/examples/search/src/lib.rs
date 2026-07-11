// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Example search plugin for the Aggrega Wasm runtime.
//!
//! It answers a ranked full-text query over a small inverted index the
//! guest embeds at build time. `describe()` self-reports plugin_type
//! "search", declares the search export, and stamps the index format and
//! version the guest reads; the search export tokenises the query, scores
//! each document by how often the query terms occur in it, and returns the
//! matches in descending score order with a document-id tie-break so
//! identical input yields identical output.
//!
//! The index the guest embeds is index-only by design: per document it
//! carries the tokens the query is matched against plus the display fields
//! a result needs — the title, an in-document anchor, and a short snippet.
//! It does NOT carry full page bodies; a consumer that wants a matched
//! document's body fetches it from the content bundle by the returned
//! document id, keeping the guest small and its query surface offline. The
//! search export declares no host functions, so the guest reaches no host
//! data surface and answers every query from its embedded index alone.

use aggrega_pdk_rust::{SearchInput, SearchOutput, SearchResult};
use extism_pdk::{plugin_fn, FnResult};

/// The static `describe()` self-report: a v1alpha1 `search` plugin whose
/// `search` export the host validates before dispatch. The description
/// names the index format and version the guest is built against so a
/// reader of `describe()` sees the exact index contract the guest reads.
const PLUGIN_DESCRIPTOR: &str = r#"{"abi_version":"v1alpha1","plugin_type":"search","description":"Ranked full-text search over an embedded inverted index (format aggrega.search.index/v1).","exports":[{"name":"search","description":"Answer a ranked full-text query over the embedded index.","stability":"stable"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}"#;

/// The one field filter this fixture interprets: a value under this key
/// restricts the result set to documents whose id begins with the value.
/// The search socket leaves the filter vocabulary to the guest; a filter
/// under any other key matches nothing here and is otherwise ignored.
const PREFIX_FILTER_KEY: &str = "prefix";

/// One document in the embedded index. `terms` are the tokens the query is
/// matched against — the index — separate from the title, anchor, and
/// snippet, which are the display fields a result carries. No full page
/// body is embedded: the id is the handle a consumer resolves the body by.
struct IndexedDoc {
    id: &'static str,
    title: &'static str,
    anchor: &'static str,
    snippet: &'static str,
    terms: &'static [&'static str],
}

/// The fixture's embedded index. A handful of documents whose terms let a
/// query like "compiler" or "compiler render" produce a stable ranked
/// order, so an end-to-end test can assert an exact result sequence.
const CORPUS: &[IndexedDoc] = &[
    IndexedDoc {
        id: "architecture/invariants",
        title: "Architectural Invariants",
        anchor: "the-compiler-is-dumb",
        snippet: "The compiler is a dumb, unopinionated text router.",
        terms: &["compiler", "invariant", "router", "text", "dumb", "aot"],
    },
    IndexedDoc {
        id: "architecture/pipeline",
        title: "Compilation Pipeline",
        anchor: "phases",
        snippet: "The compiler pipeline renders the graph in ordered phases.",
        terms: &["compiler", "pipeline", "phase", "render", "graph"],
    },
    IndexedDoc {
        id: "language-reference/component",
        title: "Component Entity",
        anchor: "schema",
        snippet: "A Component declares its data pillars under a schema.",
        terms: &["component", "schema", "values", "infra", "provides"],
    },
    IndexedDoc {
        id: "language-reference/recipe",
        title: "Recipe Entity",
        anchor: "values-mapping",
        snippet: "A Recipe maps values into provider render input.",
        terms: &["recipe", "values", "mapping", "provider", "render"],
    },
];

/// Lowercase `s` and split it into alphanumeric tokens, discarding
/// punctuation and empties. The query grammar is the guest's own; the host
/// passes the query string through verbatim and does not parse it.
fn tokenize(s: &str) -> Vec<String> {
    s.to_lowercase()
        .split(|c: char| !c.is_ascii_alphanumeric())
        .filter(|t| !t.is_empty())
        .map(|t| t.to_string())
        .collect()
}

/// The host calls `describe()` once at load to read the plugin's static
/// self-report before dispatching `search`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(PLUGIN_DESCRIPTOR.to_string())
}

#[plugin_fn]
pub fn search(input: String) -> FnResult<String> {
    let parsed: SearchInput = serde_json::from_str(&input)?;

    let prefix = parsed.filters.get(PREFIX_FILTER_KEY);
    let query_tokens = tokenize(&parsed.query);

    // Score every document by the total occurrences of the query's tokens
    // in its terms. A document with no query-term match scores zero and is
    // dropped, so the result set is the matching documents alone.
    let mut hits: Vec<(usize, f64)> = Vec::new();
    for (i, doc) in CORPUS.iter().enumerate() {
        if let Some(p) = prefix {
            if !doc.id.starts_with(p.as_str()) {
                continue;
            }
        }
        let mut score = 0.0_f64;
        for qt in &query_tokens {
            for term in doc.terms {
                if *term == qt {
                    score += 1.0;
                }
            }
        }
        if score > 0.0 {
            hits.push((i, score));
        }
    }

    // Rank by descending score, breaking ties on ascending document id so
    // the output order is a deterministic function of the input — identical
    // input yields identical order, the property a diff-based consumer
    // relies on.
    hits.sort_by(|a, b| {
        b.1.partial_cmp(&a.1)
            .unwrap_or(std::cmp::Ordering::Equal)
            .then_with(|| CORPUS[a.0].id.cmp(CORPUS[b.0].id))
    });

    // A positive limit caps the result set at that size; zero or absent
    // returns every match.
    if parsed.limit > 0 && hits.len() > parsed.limit as usize {
        hits.truncate(parsed.limit as usize);
    }

    let results: Vec<SearchResult> = hits
        .into_iter()
        .map(|(i, score)| {
            let doc = &CORPUS[i];
            SearchResult {
                doc_id: doc.id.to_string(),
                title: doc.title.to_string(),
                anchor: doc.anchor.to_string(),
                snippet: doc.snippet.to_string(),
                score,
            }
        })
        .collect();

    let out = SearchOutput { results };
    Ok(serde_json::to_string(&out)?)
}
