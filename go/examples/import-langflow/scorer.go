// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import (
	"sort"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// The confidence scorer is deterministic and holds no model: it weighs a source
// node against each embedded candidate component through five independent
// signals, combined as a fixed weighted sum. Identical input yields byte-identical
// scores, and ranking ties break by candidate name, so the ordering is stable
// across runs. No language model participates in scoring; a downstream renderer
// may narrate the decision record, but it never sets a score.
const (
	weightTable        = 0.50
	weightCategory     = 0.20
	weightDomainLabels = 0.15
	weightShape        = 0.10
	weightName         = 0.05

	tierHigh   = "HIGH"
	tierMedium = "MEDIUM"
	tierLow    = "LOW"
	tierNone   = "NONE"

	thresholdHigh   = 0.85
	thresholdMedium = 0.50
	thresholdLow    = 0.20

	maxCandidates = 4
)

// scoreNode ranks the embedded candidate components for one source node. It
// returns the candidates whose score is positive, ordered by score descending and
// then by component name ascending so ties resolve deterministically, capped to a
// bounded head so the decision record stays compact. A node that matches nothing
// returns no candidates, which the emitter treats as the draft-new fallback.
func scoreNode(overrides mappingOverrides, format string, node sourceNode) []aggrega.ImportCandidate {
	category := inferCategory(overrides, format, node)
	traits := nodeTraits(overrides, format, node)
	vendor := nodeVendor(overrides, format, node)
	nodeShape := append(append([]string{}, node.fields...), node.secrets...)

	var candidates []aggrega.ImportCandidate
	for i := range embeddedCatalog {
		c := &embeddedCatalog[i]
		var sig aggrega.CandidateSignals

		if e, ok := lookupMapping(overrides, format, node.typ); ok && e.component == c.name {
			sig.Table = weightTable
		}
		if category != "" && category == c.category {
			sig.Category = weightCategory
		}
		sig.DomainLabels = round4(weightDomainLabels * traitMatchFraction(traits, c.domainLabels))
		candidateShape := append(append([]string{}, c.values...), c.secrets...)
		sig.Shape = round4(weightShape * jaccard(nodeShape, candidateShape))
		sig.Name = round4(weightName * bestNameRatio(vendor, c.name))

		// The score is the sum of the rounded per-signal contributions, so the
		// breakdown the decision record shows adds up exactly to the score and both
		// are stable, byte-identical values across runs.
		score := sig.Table + sig.Category + sig.DomainLabels + sig.Shape + sig.Name
		if score <= 0 {
			continue
		}
		candidates = append(candidates, aggrega.ImportCandidate{
			ComponentName: c.name,
			Score:         round4(score),
			Tier:          tierFor(round4(score)),
			Signals:       sig,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].ComponentName < candidates[j].ComponentName
	})
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}
	return candidates
}

// traitMatchFraction returns the fraction of a node's implied traits that the
// candidate's domain labels agree with. A trait agrees when the candidate carries
// the same label key with the same value. No traits yields 0, so a node with no
// discernible traits draws no domain-label contribution.
func traitMatchFraction(traits, labels map[string]string) float64 {
	if len(traits) == 0 {
		return 0
	}
	matched := 0
	for k, v := range traits {
		if lv, ok := labels[k]; ok && lv == v {
			matched++
		}
	}
	return float64(matched) / float64(len(traits))
}

// bestNameRatio returns the strongest name-similarity between a node's vendor
// token and any token of the candidate name's last segment. An empty vendor
// yields 0.
func bestNameRatio(vendor, componentName string) float64 {
	if vendor == "" {
		return 0
	}
	best := 0.0
	for _, tok := range lastSegmentTokens(componentName) {
		if r := levenshteinRatio(vendor, tok); r > best {
			best = r
		}
	}
	return best
}

// inferCategory returns the category a node belongs to: the mapping's category
// when the node-type is mapped, otherwise a keyword read of the node-type. The
// keyword read lets the category signal contribute for an unmapped node without
// forcing a table entry.
func inferCategory(overrides mappingOverrides, format string, node sourceNode) string {
	if e, ok := lookupMapping(overrides, format, node.typ); ok {
		return e.category
	}
	switch {
	case containsFold(node.typ, "embed"):
		return "ai-model"
	case containsFold(node.typ, "model") || containsFold(node.typ, "llm") || containsFold(node.typ, "chat"):
		return "ai-model"
	case containsFold(node.typ, "tool") || containsFold(node.typ, "request") || containsFold(node.typ, "retriev") || containsFold(node.typ, "search"):
		return "ai-tool"
	case containsFold(node.typ, "agent") || containsFold(node.typ, "flow"):
		return "ai-agent"
	default:
		return ""
	}
}

// nodeTraits returns the refining traits a node implies: the mapping's traits when
// mapped, otherwise none. Traits are deliberately not inferred for an unmapped
// node, so an unmapped node's candidates separate on category and shape rather
// than on guessed traits.
func nodeTraits(overrides mappingOverrides, format string, node sourceNode) map[string]string {
	if e, ok := lookupMapping(overrides, format, node.typ); ok {
		return e.traits
	}
	return nil
}

// nodeVendor returns the vendor token a node carries: the mapping's vendor when
// mapped, otherwise empty. An unmapped node draws no name-signal contribution.
func nodeVendor(overrides mappingOverrides, format string, node sourceNode) string {
	if e, ok := lookupMapping(overrides, format, node.typ); ok {
		return e.vendor
	}
	return ""
}

func tierFor(score float64) string {
	switch {
	case score >= thresholdHigh:
		return tierHigh
	case score >= thresholdMedium:
		return tierMedium
	case score >= thresholdLow:
		return tierLow
	default:
		return tierNone
	}
}

// round4 rounds a score to four decimal places so the weighted sum is a stable,
// byte-identical value across runs rather than a float tail that could differ.
func round4(f float64) float64 {
	const scale = 10000
	if f >= 0 {
		return float64(int64(f*scale+0.5)) / scale
	}
	return float64(int64(f*scale-0.5)) / scale
}
