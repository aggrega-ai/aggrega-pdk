// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// runImport turns one import request into the entities, decisions, diagnostics,
// and files it produced. It is the pure core of the guest: it takes the decoded
// request and a parse callback — the guest's only channel to a source tree, which
// the host mediates — and returns the assembled output, holding no state and
// touching no ambient resource. The main package's export wires the real host
// parse callback into this function; a test wires a local one.
//
// The flow is: decode the settings, walk each source into nodes and edges through
// the host-mediated parse, score every node against the embedded candidates,
// decide each node's disposition from its top candidate and the reuse mode, and
// render the shared decision record into files through the selected renderer.
func runImport(in aggrega.ImportInput, parse func(format string, source []byte) (json.RawMessage, error)) aggrega.ImportOutput {
	settings := parseSettings(in.Settings)
	format := in.Format

	sources := in.Sources
	if len(sources) == 0 {
		sources = []aggrega.ImportSource{{Name: "", Bytes: in.Bytes}}
	}
	multiSource := len(sources) > 1

	var out aggrega.ImportOutput
	agg := newAggregateBuilder(settings, format)
	usedNames := map[string]bool{}

	for _, src := range sources {
		tree, err := parse(syntacticFormat(format), src.Bytes)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
				Severity:  "error",
				Code:      "parse-failed",
				Message:   fmt.Sprintf("could not parse source %q as %s: %s", src.Name, format, err.Error()),
				SourceRef: src.Name,
			})
			continue
		}
		nodes, edges := walkTree(format, tree)
		if len(nodes) == 0 {
			out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
				Severity:  "warning",
				Code:      "no-nodes",
				Message:   fmt.Sprintf("source %q carried no recognisable %s nodes", src.Name, format),
				SourceRef: src.Name,
			})
			continue
		}

		idToKey := map[string]string{}
		for _, node := range nodes {
			ref := node.id
			if multiSource {
				ref = src.Name + "#" + node.id
			}
			key := uniqueKey(usedNames, kebab(node.id))
			idToKey[node.id] = key

			decision, entity := decideNode(settings, format, node, ref)
			out.Decisions = append(out.Decisions, decision)
			out.Entities = append(out.Entities, entity)
			agg.addComponent(key, decision, entity)
		}
		for _, e := range edges {
			consumer, ok1 := idToKey[e.target]
			producer, ok2 := idToKey[e.source]
			if ok1 && ok2 {
				agg.addEdge(consumer, producer)
			}
		}
	}

	// The aggregate wiring the drafted and referenced components together is the
	// last entity, emitted after the components it composes.
	out.Entities = append(out.Entities, agg.entity())

	out.Files = render(settings, out.Entities, out.Decisions)
	return out
}

// syntacticFormat maps a semantic source format to the syntactic format the host
// parses it as. The langflow and flowise agent-definition formats are both JSON,
// so the guest asks the host for a JSON parse; the mapping is the guest's domain
// knowledge, not the host's.
func syntacticFormat(format string) string {
	switch format {
	case "langflow", "flowise":
		return "json"
	default:
		return "json"
	}
}

// decideNode scores a node, picks its disposition from the top candidate and the
// reuse mode, and returns the decision record plus the entity emitted for it. A
// high-confidence match under auto-apply mode is referenced; every other outcome
// drafts a new component, carrying a hint whose strength tracks the match tier.
func decideNode(settings importSettings, format string, node sourceNode, ref string) (aggrega.ImportDecision, aggrega.EmittedEntity) {
	candidates := scoreNode(settings.MappingOverrides, format, node)
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
		SourceNode: aggrega.DecisionSourceNode{ID: ref, Type: node.typ, ParamsSummary: paramsSummary(node)},
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
	draftName := draftComponentName(settings, format, node)
	body := draftComponentBody(draftName, format, node)
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

func paramsSummary(node sourceNode) string {
	var parts []string
	if len(node.fields) > 0 {
		parts = append(parts, "fields: "+strings.Join(node.fields, ", "))
	}
	if len(node.secrets) > 0 {
		parts = append(parts, "secrets: "+strings.Join(node.secrets, ", "))
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
func draftComponentName(settings importSettings, format string, node sourceNode) string {
	category := inferCategory(settings.MappingOverrides, format, node)
	if category == "" {
		category = "component"
	}
	name := kebab(node.typ)
	if name == "" {
		name = kebab(node.id)
	}
	return settings.TargetNamespace + "/" + category + "/" + name
}

// draftComponentBody renders a drafted component as deterministic YAML: the
// author's namespace-qualified name, the ontology type inferred from its category,
// its category label, its non-secret fields as string value slots, and its
// credential-named fields as blackbox secret slots. The body is hand-built rather
// than marshalled so key order is fixed and the output is byte-identical across
// runs.
func draftComponentBody(name, format string, node sourceNode) string {
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
		b.WriteString("    component.aggrega.ai/category: " + category + "\n")
	}

	nonSecret := subtract(node.fields, node.secrets)
	if len(nonSecret) == 0 && len(node.secrets) == 0 {
		b.WriteString("spec: {}\n")
		return b.String()
	}
	b.WriteString("spec:\n")
	if len(node.secrets) > 0 {
		b.WriteString("  secretsSchema:\n")
		for _, s := range node.secrets {
			b.WriteString("    " + s + ":\n")
			b.WriteString("      required: true\n")
			b.WriteString("      inject: []\n")
			b.WriteString("      description: Credential slot inferred from the source node; the recipe controls delivery.\n")
		}
	}
	if len(nonSecret) > 0 {
		b.WriteString("  schema:\n")
		b.WriteString("    values:\n")
		for _, f := range nonSecret {
			b.WriteString("      " + f + ": { type: string }\n")
		}
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

// subtract returns the elements of a not present in b, preserving a's order.
func subtract(a, b []string) []string {
	drop := make(map[string]struct{}, len(b))
	for _, s := range b {
		drop[s] = struct{}{}
	}
	var out []string
	for _, s := range a {
		if _, ok := drop[s]; !ok {
			out = append(out, s)
		}
	}
	return out
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

// entity renders the aggregate as a draft EmittedEntity carrying hand-built YAML.
func (a *aggregateBuilder) entity() aggrega.EmittedEntity {
	var b strings.Builder
	b.WriteString("apiVersion: aggrega.ai/v1alpha1\n")
	b.WriteString("kind: Aggregate\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: " + a.settings.TargetNamespace + "/flow\n")
	b.WriteString("  version: 0.1.0\n")
	b.WriteString("  description: Draft aggregate imported from a " + a.format + " source.\n")
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
