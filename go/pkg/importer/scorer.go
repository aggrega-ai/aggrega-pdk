// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"sort"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// The confidence scorer is deterministic and holds no model: it weighs a source
// node against each candidate component through five independent signals,
// combined as a fixed weighted sum. Identical input yields byte-identical scores,
// and ranking ties break by candidate name, so the ordering is stable across
// runs. No language model participates in scoring; a downstream renderer may
// narrate the decision record, but it never sets a score.
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

// scoreNode ranks the candidate components for one source node. It returns the
// candidates whose score is positive, ordered by score descending and then by
// component name ascending so ties resolve deterministically, capped to a bounded
// head so the decision record stays compact. A node that matches nothing returns
// no candidates, which the decision treats as the draft-new fallback.
func scoreNode(table mappingTable, candidates []candidate, node Node) []aggrega.ImportCandidate {
	category := inferCategory(table, node)
	traits := nodeTraits(table, node)
	vendor := nodeVendor(table, node)
	nodeShape := append(append([]string{}, node.Fields...), node.Secrets...)

	var ranked []aggrega.ImportCandidate
	for i := range candidates {
		c := &candidates[i]
		var sig aggrega.CandidateSignals

		if e, ok := table.lookup(node.Type); ok && e.Component == c.name {
			sig.Table = weightTable
		}
		if category != "" && category == c.category {
			sig.Category = weightCategory
		}
		sig.DomainLabels = round4(weightDomainLabels * traitMatchFraction(traits, c.labels))
		sig.Shape = round4(weightShape * jaccard(nodeShape, c.values))
		sig.Name = round4(weightName * bestNameRatio(vendor, c.name))

		// The score is the sum of the rounded per-signal contributions, so the
		// breakdown the decision record shows adds up exactly to the score and both
		// are stable, byte-identical values across runs.
		score := sig.Table + sig.Category + sig.DomainLabels + sig.Shape + sig.Name
		if score <= 0 {
			continue
		}
		ranked = append(ranked, aggrega.ImportCandidate{
			ComponentName: c.name,
			Score:         round4(score),
			Tier:          tierFor(round4(score)),
			Signals:       sig,
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].ComponentName < ranked[j].ComponentName
	})
	if len(ranked) > maxCandidates {
		ranked = ranked[:maxCandidates]
	}
	return ranked
}

// traitMatchFraction returns the fraction of a node's implied traits that the
// candidate's labels agree with. A trait agrees when the candidate carries the
// same label key with the same value; a label at a key the node implies no trait
// for plays no part. No traits yields 0, so a node with no discernible traits
// draws no domain-label contribution.
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
// when the node type is mapped, otherwise a keyword read of the node type. The
// keyword read lets the category signal contribute for an unmapped node without
// forcing a mapping.
func inferCategory(table mappingTable, node Node) string {
	if e, ok := table.lookup(node.Type); ok {
		return e.Category
	}
	switch {
	case containsFold(node.Type, "embed"):
		return "ai-model"
	case containsFold(node.Type, "model") || containsFold(node.Type, "llm") || containsFold(node.Type, "chat"):
		return "ai-model"
	case containsFold(node.Type, "tool") || containsFold(node.Type, "request") || containsFold(node.Type, "retriev") || containsFold(node.Type, "search"):
		return "ai-tool"
	case containsFold(node.Type, "agent") || containsFold(node.Type, "flow"):
		return "ai-agent"
	default:
		return ""
	}
}

// nodeTraits returns the refining traits a node implies: the mapping's traits when
// mapped, otherwise none. Traits are deliberately not inferred for an unmapped
// node, so an unmapped node's candidates separate on category and shape rather
// than on guessed traits.
func nodeTraits(table mappingTable, node Node) map[string]string {
	if e, ok := table.lookup(node.Type); ok {
		return e.Traits
	}
	return nil
}

// nodeVendor returns the vendor token a node carries: the mapping's vendor when
// mapped, otherwise empty. An unmapped node draws no name-signal contribution.
func nodeVendor(table mappingTable, node Node) string {
	if e, ok := table.lookup(node.Type); ok {
		return e.Vendor
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
