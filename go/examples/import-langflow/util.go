// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import (
	"bytes"
	"encoding/json"
	"strings"
)

// jsonObjectKeys returns the keys of a JSON object in the textual order the
// document presents them, using a streaming token walk so the order is preserved
// (decoding into a Go map would lose it). The host's parser emits object keys in
// sorted order, so this order is deterministic across runs of the same source.
func jsonObjectKeys(raw json.RawMessage) []string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	var keys []string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return keys
		}
		key, ok := keyTok.(string)
		if !ok {
			return keys
		}
		keys = append(keys, key)
		// Skip the value, whatever its shape.
		if err := skipValue(dec); err != nil {
			return keys
		}
	}
	return keys
}

// skipValue consumes one JSON value from dec, descending through nested objects
// and arrays so the decoder lands on the next key.
func skipValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); ok && (d == '{' || d == '[') {
		depth := 1
		for depth > 0 {
			t, err := dec.Token()
			if err != nil {
				return err
			}
			if dd, ok := t.(json.Delim); ok {
				switch dd {
				case '{', '[':
					depth++
				case '}', ']':
					depth--
				}
			}
		}
	}
	return nil
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// jaccard returns the Jaccard similarity of two string sets: the size of their
// intersection over the size of their union. Empty-over-empty is 0.
func jaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(a))
	for _, s := range a {
		set[strings.ToLower(s)] = struct{}{}
	}
	inter := 0
	union := make(map[string]struct{}, len(a)+len(b))
	for _, s := range a {
		union[strings.ToLower(s)] = struct{}{}
	}
	for _, s := range b {
		ls := strings.ToLower(s)
		if _, ok := set[ls]; ok {
			inter++
		}
		union[ls] = struct{}{}
	}
	if len(union) == 0 {
		return 0
	}
	return float64(inter) / float64(len(union))
}

// levenshteinRatio returns a name-similarity score in [0,1]: 1 for an exact
// match, falling toward 0 as the edit distance grows relative to the longer
// string. An empty operand yields 0, so a candidate with no vendor token never
// draws a spurious name-signal contribution.
func levenshteinRatio(a, b string) float64 {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	d := levenshtein(a, b)
	longest := len(a)
	if len(b) > longest {
		longest = len(b)
	}
	return 1 - float64(d)/float64(longest)
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// lastSegmentTokens splits a component name's last path segment into tokens on
// hyphens, so a name like core/ai-model/openai-embedding yields
// {openai, embedding} — the tokens the name signal scores a node's vendor against.
func lastSegmentTokens(name string) []string {
	seg := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		seg = name[i+1:]
	}
	return strings.Split(seg, "-")
}
