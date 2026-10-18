// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// decideNode scores a node against the catalog's candidates, picks its
// disposition from the top candidate and the reuse mode, and returns the decision
// record plus the entity emitted for it. A high-confidence match under auto-apply
// mode is referenced; every other outcome drafts a new component, carrying a hint
// whose strength tracks the match tier.
func decideNode(settings importSettings, table mappingTable, catalog []candidate, node Node, ref string) (aggrega.ImportDecision, aggrega.EmittedEntity) {
	candidates := scoreNode(table, catalog, node)
	decisionRef := "decision:" + ref

	tier := tierNone
	var top aggrega.ImportCandidate
	if len(candidates) > 0 {
		top = candidates[0]
		tier = top.Tier
	}

	autoApply := settings.ReuseMode == reuseModeAutoApply &&
		len(candidates) > 0 && tier == tierHigh && top.Score >= settings.ReuseThreshold

	decision := aggrega.ImportDecision{
		SourceNode: aggrega.DecisionSourceNode{ID: ref, Type: node.Type, ParamsSummary: paramsSummary(node)},
		Candidates: candidates,
	}

	if autoApply {
		entity := aggrega.EmittedEntity{
			Kind:        "Component",
			Name:        top.ComponentName,
			Mode:        "reference",
			SourceRef:   ref,
			DecisionRef: decisionRef,
		}
		decision.Chosen = &aggrega.DecisionChoice{Target: top.ComponentName, Mode: "reference", Tier: tier}
		decision.Rationale = fmt.Sprintf("referenced %s at score %.4f (%s) under auto-apply", top.ComponentName, top.Score, tier)
		decision.SwapHint = runnerUpHint(candidates)
		return decision, entity
	}

	// Every non-auto-apply outcome drafts a new component; the hint strength
	// tracks the tier so a strong match is loudly suggested and a weak one is only
	// recorded.
	draftName := draftComponentName(settings, table, node)
	body := draftComponentBody(draftName, node)
	entity := aggrega.EmittedEntity{
		Kind:        "Component",
		Name:        draftName,
		Mode:        "draft",
		Body:        body,
		SourceRef:   ref,
		DecisionRef: decisionRef,
	}
	decision.Chosen = &aggrega.DecisionChoice{Target: draftName, Mode: "draft", Tier: tier}

	switch tier {
	case tierHigh:
		decision.Rationale = fmt.Sprintf("drafted a new component; %s is a strong match at score %.4f but hints-only mode references nothing", top.ComponentName, top.Score)
		decision.SwapHint = fmt.Sprintf("resembles %s (score %.4f); reference it by setting type: %s", top.ComponentName, top.Score, top.ComponentName)
	case tierMedium:
		decision.Rationale = fmt.Sprintf("drafted a new component; %s is a partial match at score %.4f", top.ComponentName, top.Score)
		decision.SwapHint = fmt.Sprintf("resembles %s (score %.4f); consider referencing it", top.ComponentName, top.Score)
	case tierLow:
		decision.Rationale = fmt.Sprintf("drafted a new component; the closest candidate %s scored only %.4f", top.ComponentName, top.Score)
	default:
		decision.Rationale = "drafted a new component; no candidate scored above the draft-new floor"
	}
	return decision, entity
}

// runnerUpHint describes how to swap an auto-applied reference for its next-best
// alternative, or reports that there is no alternative.
func runnerUpHint(candidates []aggrega.ImportCandidate) string {
	if len(candidates) < 2 {
		return "no alternative candidate scored above zero"
	}
	r := candidates[1]
	return fmt.Sprintf("swap to %s (score %.4f) by setting type: %s", r.ComponentName, r.Score, r.ComponentName)
}

func paramsSummary(node Node) string {
	var parts []string
	if len(node.Fields) > 0 {
		parts = append(parts, "fields: "+strings.Join(node.Fields, ", "))
	}
	if len(node.Secrets) > 0 {
		parts = append(parts, "secrets: "+strings.Join(node.Secrets, ", "))
	}
	if len(parts) == 0 {
		return "no parameters"
	}
	return strings.Join(parts, "; ")
}

// draftComponentName builds a drafted component's fully-qualified name under the
// author's namespace: <ns>/<category>/<name>. The namespace never collides with a
// protected one; the category falls back to a neutral segment when the node's
// category cannot be inferred.
func draftComponentName(settings importSettings, table mappingTable, node Node) string {
	category := inferCategory(table, node)
	if category == "" {
		category = "component"
	}
	name := kebab(node.Type)
	if name == "" {
		name = kebab(node.ID)
	}
	return settings.TargetNamespace + "/" + category + "/" + name
}

// draftComponentBody renders a drafted component as deterministic YAML: the
// author's namespace-qualified name, the ontology type inferred from its category,
// its category label, its value fields as string value slots, and the fields the
// source marks as credentials as blackbox secret slots. The body is hand-built
// rather than marshalled so key order is fixed and the output is byte-identical
// across runs.
//
// The value slots are always written as a closed object whose fields sit under
// an explicit properties block. A source field may carry any name, including one
// the compact schema form reads as a keyword when it appears directly under
// values; under properties every key is a field name, so one shape compiles
// whatever the source calls its fields, and it compiles to the same closed object
// the compact form writes for fields that collide with no keyword.
func draftComponentBody(name string, node Node) string {
	category := categoryFromName(name)
	var b strings.Builder
	b.WriteString("apiVersion: aggrega.ai/v1alpha1\n")
	b.WriteString("kind: Component\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: " + name + "\n")
	b.WriteString("  version: 0.1.0\n")
	b.WriteString("  labels:\n")
	b.WriteString("    component.aggrega.ai/type: " + categoryOntology(category) + "\n")
	if category != "component" {
		b.WriteString("    " + categoryLabel + ": " + category + "\n")
	}

	if len(node.Fields) == 0 && len(node.Secrets) == 0 {
		b.WriteString("spec: {}\n")
		return b.String()
	}
	b.WriteString("spec:\n")
	if len(node.Secrets) > 0 {
		b.WriteString("  featuresSchema:\n")
		for _, s := range node.Secrets {
			b.WriteString("    " + s + ":\n")
			b.WriteString("      required: true\n")
			b.WriteString("      inject: []\n")
			b.WriteString("      description: Credential slot inferred from the source node; the recipe controls delivery.\n")
		}
	}
	if len(node.Fields) > 0 {
		b.WriteString("  schema:\n")
		b.WriteString("    values:\n")
		b.WriteString("      type: object\n")
		b.WriteString("      properties:\n")
		for _, f := range node.Fields {
			b.WriteString("        " + f + ": { type: string }\n")
		}
		b.WriteString("      additionalProperties: false\n")
	}
	return b.String()
}

// render turns the shared decision record and the emitted entities into files
// through the selected renderer. The human renderer chunks the record into
// per-entity drafts with inline hints; the llm renderer serves the whole record
// as one document. Both consume the identical entities and decisions — the
// renderer is the only thing that varies.
func render(settings importSettings, entities []aggrega.EmittedEntity, decisions []aggrega.ImportDecision) []aggrega.ScaffoldFile {
	if settings.RenderMode == renderModeLLM {
		return renderLLM(decisions)
	}
	return renderHuman(settings, entities, decisions)
}

// renderHuman writes one file per drafted component and one aggregate file,
// threading each node's resemblance hint into the drafted file as an inline
// comment when inline hints are enabled. A referenced component emits no body
// file — it names an existing entity — so only its reference in the aggregate
// records it.
func renderHuman(settings importSettings, entities []aggrega.EmittedEntity, decisions []aggrega.ImportDecision) []aggrega.ScaffoldFile {
	byDecisionRef := map[string]aggrega.ImportDecision{}
	for _, d := range decisions {
		byDecisionRef["decision:"+d.SourceNode.ID] = d
	}

	var files []aggrega.ScaffoldFile
	for _, e := range entities {
		switch {
		case e.Kind == "Aggregate":
			files = append(files, aggrega.ScaffoldFile{Path: "aggregate.yaml", Content: e.Body})
		case e.Kind == "Component" && e.Mode == "draft":
			content := e.Body
			if settings.emitInlineHints() {
				if d, ok := byDecisionRef[e.DecisionRef]; ok && d.SwapHint != "" && hintIsInline(d.Chosen) {
					content = "# " + d.SwapHint + "\n" + content
				}
			}
			files = append(files, aggrega.ScaffoldFile{Path: "components/" + fileSlug(e.Name) + ".yaml", Content: content})
		}
	}
	return files
}

// hintIsInline reports whether a decision's hint is loud enough to write into the
// drafted file. High and medium matches earn an inline comment; low and none
// stay in the structured record only, to avoid comment noise.
func hintIsInline(chosen *aggrega.DecisionChoice) bool {
	return chosen != nil && (chosen.Tier == tierHigh || chosen.Tier == tierMedium)
}

// renderLLM serves the whole decision record as one document for a downstream
// language model to narrate or act on. It is a stub-level renderer: it serialises
// the shared record deterministically rather than calling a model. It consumes
// the identical decisions the human renderer does — the swap point is the
// renderer alone.
func renderLLM(decisions []aggrega.ImportDecision) []aggrega.ScaffoldFile {
	raw, err := json.Marshal(decisions)
	if err != nil {
		return nil
	}
	return []aggrega.ScaffoldFile{{Path: "import-decisions.json", Content: string(raw)}}
}

// uniqueKey returns key, or key with a numeric suffix, so no two components share
// a key. It records the returned key so a later call sees it taken.
func uniqueKey(used map[string]bool, key string) string {
	if key == "" {
		key = "node"
	}
	candidate := key
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s-%d", key, i)
	}
	used[candidate] = true
	return candidate
}

// categoryOntology maps a component category to its reserved ontology type value.
func categoryOntology(category string) string {
	switch category {
	case "ai-model":
		return "resource"
	case "ai-tool":
		return "function"
	case "ai-agent":
		return "server"
	default:
		return "resource"
	}
}

// categoryFromName recovers the category segment of a <ns>/<category>/<name> name.
func categoryFromName(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) >= 3 {
		return parts[len(parts)-2]
	}
	return "component"
}

// fileSlug turns a component name into a filesystem-safe slug by replacing path
// separators with hyphens.
func fileSlug(name string) string {
	return strings.ReplaceAll(name, "/", "-")
}

// kebab lowercases a source identifier and replaces every run of characters
// outside [a-z0-9] with a single hyphen, trimming leading and trailing hyphens,
// so the result is a valid component key and name segment.
func kebab(s string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// aggregateBuilder assembles the drafted aggregate that wires the emitted
// components together. It records components in emission order and edges as they
// arrive, then renders deterministic YAML on demand.
type aggregateBuilder struct {
	settings  importSettings
	format    string
	order     []string
	instance  map[string]string   // component key -> its type: value
	tierByKey map[string]string   // component key -> chosen tier
	hintByKey map[string]string   // component key -> swap hint
	usesByKey map[string][]string // component key -> producer keys it uses
}

func newAggregateBuilder(settings importSettings, format string) *aggregateBuilder {
	return &aggregateBuilder{
		settings:  settings,
		format:    format,
		instance:  map[string]string{},
		tierByKey: map[string]string{},
		hintByKey: map[string]string{},
		usesByKey: map[string][]string{},
	}
}

func (a *aggregateBuilder) addComponent(key string, d aggrega.ImportDecision, e aggrega.EmittedEntity) {
	a.order = append(a.order, key)
	// The instance type is the entity's name either way: a referenced component's
	// catalog name, or a drafted component's fully-qualified name.
	a.instance[key] = e.Name
	if d.Chosen != nil {
		a.tierByKey[key] = d.Chosen.Tier
	}
	a.hintByKey[key] = d.SwapHint
}

func (a *aggregateBuilder) addEdge(consumer, producer string) {
	a.usesByKey[consumer] = append(a.usesByKey[consumer], producer)
}

// draftDescription is the metadata.description of an Aggregate an import drafts
// from a source of format, in either layout. It names the format after the
// article rather than in its place, so the sentence reads as English whatever
// the format is called.
func draftDescription(format string) string {
	return "Draft Aggregate imported from a source in the " + format + " format."
}

// entity renders the aggregate as a draft EmittedEntity carrying hand-built YAML.
func (a *aggregateBuilder) entity() aggrega.EmittedEntity {
	var b strings.Builder
	b.WriteString("apiVersion: aggrega.ai/v1alpha1\n")
	b.WriteString("kind: Aggregate\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: " + a.settings.TargetNamespace + "/flow\n")
	b.WriteString("  version: 0.1.0\n")
	b.WriteString("  description: " + draftDescription(a.format) + "\n")
	b.WriteString("spec:\n")
	b.WriteString("  components:\n")
	for _, key := range a.order {
		if a.settings.emitInlineHints() {
			if hint := a.hintByKey[key]; hint != "" && (a.tierByKey[key] == tierHigh || a.tierByKey[key] == tierMedium) {
				b.WriteString("    # " + hint + "\n")
			}
		}
		b.WriteString("    " + key + ":\n")
		b.WriteString("      type: " + a.instance[key] + "\n")
		producers := a.usesByKey[key]
		if len(producers) > 0 {
			sorted := append([]string{}, producers...)
			sort.Strings(sorted)
			b.WriteString("      uses:\n")
			for _, p := range dedup(sorted) {
				b.WriteString("        " + p + ":\n")
				b.WriteString("          link:\n")
				b.WriteString("            target: " + p + "\n")
			}
		}
	}
	return aggrega.EmittedEntity{Kind: "Aggregate", Name: a.settings.TargetNamespace + "/flow", Mode: "draft", Body: b.String()}
}

func dedup(sorted []string) []string {
	var out []string
	for i, s := range sorted {
		if i == 0 || sorted[i-1] != s {
			out = append(out, s)
		}
	}
	return out
}
