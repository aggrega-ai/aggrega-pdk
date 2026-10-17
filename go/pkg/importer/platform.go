// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// Platform describes the platform a format's flows run on — the catalog
// Component that runs a flow of the format as it is written — and how a source
// of the format is carried in it.
//
// A format carrying one is drafted, unless the layout setting asks for nodes, as
// one instance of Component per source, carrying the source in its
// DefinitionValue and the flow's identifier in its IdentifierValue. A node a
// mapping row maps, or whose best candidate is at least a MEDIUM match, is a part
// of the flow the platform reaches through a socket: it becomes a Component of
// its own — referenced or drafted exactly as the node-by-node draft decides it —
// wired to the first socket, in the order the platform's catalog entry declares
// its sockets, whose selector the labels of its Component satisfy: the
// referenced Component's labels, or, for a drafted one, its best candidate's
// where that candidate is at least a MEDIUM match. Every other node stays inside
// the flow and becomes no Component, and so does a part no socket admits, or
// whose labels nothing supplies, which the import reports. Where the
// workspace's catalog holds no Component of that name the platform is drafted
// all the same, no node becomes a part, and the import says so.
//
// The source is carried as its own bytes, with every credential value it holds
// written empty and nothing else changed, so a source holding no credential
// value is carried byte for byte. A source is read as JSON to find those values,
// so a platform's sources are JSON documents.
type Platform struct {
	// Component is the name of the catalog Component that runs a flow of the
	// format, such as core/ai-agent/langflow. The last segment of the name is the
	// key its instance is written under in the drafted Aggregate.
	Component string
	// DefinitionValue is the value of Component that carries the flow.
	DefinitionValue string
	// IdentifierValue is the value of Component naming the flow on the platform.
	IdentifierValue string
	// Identifier reads the identifier the source gives its flow off the source's
	// tree, empty where it gives none the platform takes. A flow whose source
	// gives none — or one holding a character other than a letter, a digit, a
	// dot, a hyphen, an underscore or a colon — is given one derived from its
	// carried bytes, and the draft says so. Nil reads none.
	Identifier func(tree json.RawMessage) string
	// Credentials locates every value the source's tree holds in a field the
	// format marks as a credential. Each is written empty before the source is
	// carried, and the import names each one it empties by its node and field,
	// never by its value. Nil locates none.
	Credentials func(tree json.RawMessage) []Credential
	// Match reports whether a set of labels satisfies a socket's selector, in the
	// grammar the catalog writes selectors in. The package carries no selector
	// grammar of its own; nil places no part in any socket.
	Match func(selector string, labels map[string]string) (bool, error)
	// LeftForRelease names the platform's own inputs a source cannot supply, one
	// entry each, which the drafted Aggregate leaves for the Release in a comment
	// above the platform's instance.
	LeftForRelease []string
}

// importPlatform is Import for a format describing a Platform: each source is
// drafted as the platform carrying it, as [Platform] describes.
func importPlatform(in aggrega.ImportInput, served Format, settings importSettings,
	parse func(syntax string, source []byte) (json.RawMessage, error)) aggrega.ImportOutput {
	p := served.Platform
	format := in.Format
	table := newMappingTable(settings.MappingOverrides, format, served)
	catalog := candidatesFrom(in.Catalog)
	labels := catalogLabels(in.Catalog)
	platform := catalogEntry(in.Catalog, p.Component)

	sources := sourcesOf(in)
	multiSource := len(sources) > 1

	var out aggrega.ImportOutput
	if settings.Layout != layoutPlatform {
		out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
			Severity: "warn",
			Code:     "unknown-layout",
			Message: fmt.Sprintf("the layout setting %q is neither %s nor %s, so the source is drafted as %s",
				settings.Layout, layoutPlatform, layoutNodes, layoutPlatform),
		})
	}

	draft := &platformDraft{settings: settings, format: format, platform: p}
	usedNames := map[string]bool{}
	unreadable := map[string]bool{}
	drafted := false

	for _, src := range sources {
		tree, err := parse(served.Syntax, src.Bytes)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, parseFailed(src, format, err))
			continue
		}
		nodes, _ := walk(served, format, tree)
		if len(nodes) == 0 {
			out.Diagnostics = append(out.Diagnostics, noNodes(src, format))
			continue
		}
		drafted = true

		refOf := func(id string) string {
			if multiSource {
				return src.Name + "#" + id
			}
			return id
		}
		inst := newPlatformInstance(uniqueKey(usedNames, kebab(path.Base(p.Component))), platform)
		out.Diagnostics = append(out.Diagnostics, inst.carry(p, format, src, tree, nodes, refOf)...)
		out.Entities = append(out.Entities, aggrega.EmittedEntity{
			Kind: "Component", Name: p.Component, Mode: "reference", SourceRef: src.Name,
		})

		for _, node := range nodes {
			ref := refOf(node.ID)
			decision, entity := decideNode(settings, table, catalog, node, ref)
			if !isPart(table, node, decision) {
				out.Decisions = append(out.Decisions, keptInFlow(decision, p.Component, notAPart(decision)))
				continue
			}
			target := partTarget(decision)
			if platform == nil {
				reason := fmt.Sprintf("it maps to %s, but the workspace's catalog holds no %s, so no socket of it can be read",
					target, p.Component)
				if target == "" {
					reason = fmt.Sprintf("a mapping row maps its type, but the workspace's catalog holds no %s, so no socket "+
						"of it can be read", p.Component)
				}
				out.Decisions = append(out.Decisions, keptInFlow(decision, p.Component, reason))
				out.Diagnostics = append(out.Diagnostics, partNotPlaced(ref, node, p.Component, reason))
				continue
			}
			socket, refusals := placePart(platform, labels[target], target != "", p.Match)
			for _, r := range refusals {
				if !unreadable[r.socket] {
					unreadable[r.socket] = true
					out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
						Severity: "warn",
						Code:     "socket-selector-unreadable",
						Message: fmt.Sprintf("the selector of the %s socket %s declares does not read, so no part is wired to it: %s",
							r.socket, p.Component, r.err),
					})
				}
			}
			if socket == "" {
				reason := unplacedReason(target, p.Component, p.Match != nil)
				out.Decisions = append(out.Decisions, keptInFlow(decision, p.Component, reason))
				out.Diagnostics = append(out.Diagnostics, partNotPlaced(ref, node, p.Component, reason))
				continue
			}
			key := uniqueKey(usedNames, kebab(node.ID))
			decision.Rationale += fmt.Sprintf("; wired to the %s socket of %s as %s", socket, p.Component, key)
			out.Decisions = append(out.Decisions, decision)
			out.Entities = append(out.Entities, entity)
			inst.connect(socket, key)
			inst.parts = append(inst.parts, platformPart{key: key, decision: decision, entity: entity})
		}
		draft.instances = append(draft.instances, inst)
	}

	if drafted && platform == nil {
		out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
			Severity: "warn",
			Code:     "platform-not-in-catalog",
			Message: fmt.Sprintf("the workspace's catalog holds no %s: the draft names it as the platform carrying the flow "+
				"and wires no part of the flow to it", p.Component),
		})
	}

	out.Entities = append(out.Entities, draft.entity())
	out.Files = render(settings, out.Entities, out.Decisions)
	return out
}

// catalogEntry returns the catalog view's entry for the Component named name, or
// nil where the view holds none.
func catalogEntry(view *aggrega.CatalogProjection, name string) *aggrega.CatalogComponent {
	if view == nil {
		return nil
	}
	for i := range view.Components {
		if view.Components[i].Name == name {
			return &view.Components[i]
		}
	}
	return nil
}

// catalogLabels returns the labels of every Component the catalog view holds, by
// name.
func catalogLabels(view *aggrega.CatalogProjection) map[string]map[string]string {
	out := map[string]map[string]string{}
	if view == nil {
		return out
	}
	for _, c := range view.Components {
		out[c.Name] = c.Labels
	}
	return out
}

// isPart reports whether a node is a part of the flow a socket of the platform
// may take: a mapping row maps its type, or its best candidate is at least a
// MEDIUM match.
func isPart(table mappingTable, node Node, d aggrega.ImportDecision) bool {
	if _, mapped := table.lookup(node.Type); mapped {
		return true
	}
	if len(d.Candidates) == 0 {
		return false
	}
	tier := d.Candidates[0].Tier
	return tier == tierHigh || tier == tierMedium
}

// partTarget returns the Component whose labels decide which socket a part is
// wired to: the Component it references, or the best candidate of a drafted one
// where that candidate is at least a MEDIUM match. It is empty where the part has
// neither: a part a mapping row names whose best candidate resembles it only
// weakly — the row's Component absent from the catalog — has no Component its
// placement can be read off, so it is wired to no socket rather than to the one a
// weak resemblance would pick.
func partTarget(d aggrega.ImportDecision) string {
	if d.Chosen != nil && d.Chosen.Mode == "reference" {
		return d.Chosen.Target
	}
	if len(d.Candidates) > 0 && (d.Candidates[0].Tier == tierHigh || d.Candidates[0].Tier == tierMedium) {
		return d.Candidates[0].ComponentName
	}
	return ""
}

// socketRefusal is one socket whose selector the matcher could not read.
type socketRefusal struct {
	socket string
	err    error
}

// placePart returns the first socket of the platform, in the order its catalog
// entry declares them, whose selector the labels satisfy, or "" where none does,
// with every socket whose selector the matcher refused to read. A part with no
// Component to read labels off, and any part where there is no matcher, is
// placed nowhere.
func placePart(platform *aggrega.CatalogComponent, labels map[string]string, known bool,
	match func(selector string, labels map[string]string) (bool, error)) (string, []socketRefusal) {
	if match == nil || !known {
		return "", nil
	}
	var refusals []socketRefusal
	for _, s := range platform.Sockets {
		ok, err := match(s.Selector, labels)
		if err != nil {
			refusals = append(refusals, socketRefusal{socket: s.Name, err: err})
			continue
		}
		if ok {
			return s.Name, refusals
		}
	}
	return "", refusals
}

// unplacedReason says why a part was placed in no socket of the platform.
func unplacedReason(target, platform string, matching bool) string {
	switch {
	case target == "":
		return "a mapping row maps its type to no Component the workspace's catalog holds, so no socket of " + platform +
			" can take it"
	case !matching:
		return "it maps to " + target + ", but this importer reads no socket selector, so no socket of " + platform +
			" takes it"
	default:
		return "it maps to " + target + ", which satisfies the selector of no socket " + platform + " declares"
	}
}

// notAPart says why a node is not a part of the flow a socket may take.
func notAPart(d aggrega.ImportDecision) string {
	if len(d.Candidates) == 0 {
		return "no mapping row names its type and no Component of the workspace's catalog resembles it"
	}
	top := d.Candidates[0]
	return fmt.Sprintf("no mapping row names its type and its closest candidate %s scored only %.4f (%s)",
		top.ComponentName, top.Score, top.Tier)
}

// keptInFlow turns a node's decision into the record of a node the platform's
// flow keeps: the candidates it was weighed against stay, it chooses no
// Component and offers no swap — no Component stands for it in the draft — and
// its rationale says where it went and why.
func keptInFlow(d aggrega.ImportDecision, platform, reason string) aggrega.ImportDecision {
	d.Chosen = nil
	d.SwapHint = ""
	d.Rationale = "kept in the flow " + platform + " carries, with no Component of its own: " + reason
	return d
}

// partNotPlaced reports a part of the flow no socket of the platform takes.
func partNotPlaced(ref string, node Node, platform, reason string) aggrega.Diagnostic {
	return aggrega.Diagnostic{
		Severity:  "warn",
		Code:      "part-not-placed",
		Message:   fmt.Sprintf("node %s (%s) is wired to no socket of %s: %s; it stays in the flow", ref, node.Type, platform, reason),
		SourceRef: ref,
	}
}

// platformInstance is one instance of the platform in the drafted Aggregate: the
// flow it carries, its identifier, the connections its sockets draw, and the
// parts they reach.
type platformInstance struct {
	key string
	// definition is the flow as written into the definition value, escaped and
	// with its credentials emptied; empty where the flow is not carried.
	definition string
	carried    bool
	identifier string
	// idNote, where set, says how the identifier was derived.
	idNote string
	// sockets are the platform's sockets in the order its catalog entry declares
	// them, the order its connections are written in.
	sockets []string
	uses    map[string][]string
	parts   []platformPart
}

// platformPart is one part of the flow drafted as a Component of its own.
type platformPart struct {
	key      string
	decision aggrega.ImportDecision
	entity   aggrega.EmittedEntity
}

// newPlatformInstance returns an instance keyed key of the platform whose
// catalog entry is entry, nil where the catalog holds none.
func newPlatformInstance(key string, entry *aggrega.CatalogComponent) *platformInstance {
	inst := &platformInstance{key: key, uses: map[string][]string{}}
	if entry != nil {
		for _, s := range entry.Sockets {
			inst.sockets = append(inst.sockets, s.Name)
		}
	}
	return inst
}

// connect records a connection of socket to the part keyed key.
func (inst *platformInstance) connect(socket, key string) {
	inst.uses[socket] = append(inst.uses[socket], key)
}

// wired reports whether any socket of the instance draws a connection.
func (inst *platformInstance) wired() bool {
	return len(inst.parts) > 0
}

// carry prepares the flow one source carries into the instance: its credential
// values written empty, its text escaped for the Aggregate's template reading,
// and its identifier. It returns the diagnostics the preparation raised: one per
// credential value it emptied, naming the node and the field; and, where the
// flow cannot be carried, why — a credential value the source's bytes do not
// hold where its tree does, or a character a YAML literal block cannot carry as
// written. Where the source gives its flow no identifier a draft can carry, one is
// derived from the carried bytes — or, where a credential value could not be
// located and the flow is not carried, from its nodes' identifiers, so nothing a
// credential value could reach decides it.
func (inst *platformInstance) carry(p *Platform, format string, src aggrega.ImportSource, tree json.RawMessage,
	nodes []Node, refOf func(string) string) []aggrega.Diagnostic {
	var diags []aggrega.Diagnostic
	var creds []Credential
	if p.Credentials != nil {
		creds = p.Credentials(tree)
	}
	own := ""
	if p.Identifier != nil {
		own = p.Identifier(tree)
	}
	flow := "the flow"
	if src.Name != "" {
		flow = fmt.Sprintf("the flow of source %q", src.Name)
	}
	scrubbed, emptied, err := scrubCredentials(src.Bytes, creds)
	if err != nil {
		diags = append(diags, aggrega.Diagnostic{
			Severity: "warn",
			Code:     "definition-not-carried",
			Message: fmt.Sprintf("%s is not carried in values.%s: %s, so a credential it holds could not be kept out of "+
				"the draft", flow, p.DefinitionValue, err.Error()),
			SourceRef: src.Name,
		})
		if carriedIdentifier(own) {
			inst.identifier = own
			return diags
		}
		ids := make([]string, 0, len(nodes))
		for _, n := range nodes {
			ids = append(ids, n.ID)
		}
		inst.identifier = derivedFlowID(format, []byte(strings.Join(ids, "\n")))
		inst.idNote = "derived from the identifiers of the flow's nodes, since the flow is not carried"
		return diags
	}
	for _, c := range emptied {
		diags = append(diags, aggrega.Diagnostic{
			Severity: "warn",
			Code:     "credential-emptied",
			Message: fmt.Sprintf("the value of %s, which node %s marks as a credential, is written empty in the flow the "+
				"draft carries: no credential from the source reaches the draft", c.Field, refOf(c.Node)),
			SourceRef: refOf(c.Node),
		})
	}

	text := string(scrubbed)
	if problems := blockProblems(text); len(problems) > 0 {
		for _, pr := range problems {
			diags = append(diags, aggrega.Diagnostic{
				Severity: "warn",
				Code:     "definition-not-carried",
				Message: fmt.Sprintf("%s is not carried in values.%s: it holds a %s (%d in all, the first at line %d, "+
					"column %d), which a YAML literal block cannot carry as written",
					flow, p.DefinitionValue, pr.what, pr.count, pr.line, pr.column),
				SourceRef: src.Name,
			})
		}
	} else {
		inst.definition = escapeTemplateText(text)
		inst.carried = true
	}

	switch {
	case carriedIdentifier(own):
		inst.identifier = own
	case own == "":
		inst.identifier = derivedFlowID(format, scrubbed)
		inst.idNote = "derived from the exported flow, which gives no identifier of its own"
	default:
		inst.identifier = derivedFlowID(format, scrubbed)
		inst.idNote = "derived from the exported flow: its own identifier holds characters a draft does not write as they are"
	}
	return diags
}

// platformDraft assembles the drafted Aggregate of a platform draft: each
// instance of the platform, followed by the parts it is wired to.
type platformDraft struct {
	settings  importSettings
	format    string
	platform  *Platform
	instances []*platformInstance
}

// entity renders the Aggregate as a draft EmittedEntity carrying hand-built
// YAML, so key order is fixed and the output is byte-identical across runs.
func (d *platformDraft) entity() aggrega.EmittedEntity {
	p := d.platform
	var b strings.Builder
	b.WriteString("apiVersion: aggrega.ai/v1alpha1\n")
	b.WriteString("kind: Aggregate\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: " + d.settings.TargetNamespace + "/flow\n")
	b.WriteString("  version: 0.1.0\n")
	b.WriteString("  description: Draft aggregate imported from a " + d.format + " source.\n")
	b.WriteString("spec:\n")
	b.WriteString("  components:\n")
	for _, inst := range d.instances {
		if len(p.LeftForRelease) > 0 || inst.wired() {
			b.WriteString("    # Left for the Release, since the source cannot supply it:\n")
			for _, item := range p.LeftForRelease {
				b.WriteString("    #   - " + item + "\n")
			}
			if inst.wired() {
				b.WriteString("    #   - the credential each connection under uses presents to the part it reaches, " +
					"attached on that connection\n")
			}
		}
		b.WriteString("    " + inst.key + ":\n")
		b.WriteString("      type: " + p.Component + "\n")
		b.WriteString("      values:\n")
		if inst.idNote != "" {
			b.WriteString("        # " + p.IdentifierValue + " is " + inst.idNote + ".\n")
		}
		b.WriteString("        " + p.IdentifierValue + ": '" + inst.identifier + "'\n")
		if inst.carried {
			b.WriteString("        " + p.DefinitionValue + ": " + literalBlock(inst.definition, 8))
		} else {
			b.WriteString("        # " + p.DefinitionValue + " is left unset: the import's diagnostics say why the flow " +
				"is not carried.\n")
		}
		if inst.wired() {
			b.WriteString("      uses:\n")
			for _, socket := range inst.sockets {
				if len(inst.uses[socket]) == 0 {
					continue
				}
				b.WriteString("        " + socket + ":\n")
				for _, key := range inst.uses[socket] {
					b.WriteString("          " + key + ":\n")
					b.WriteString("            target: " + key + "\n")
				}
			}
		}
		for _, part := range inst.parts {
			if d.settings.emitInlineHints() && part.decision.SwapHint != "" && hintIsInline(part.decision.Chosen) {
				b.WriteString("    # " + part.decision.SwapHint + "\n")
			}
			b.WriteString("    " + part.key + ":\n")
			b.WriteString("      type: " + part.entity.Name + "\n")
		}
	}
	return aggrega.EmittedEntity{Kind: "Aggregate", Name: d.settings.TargetNamespace + "/flow", Mode: "draft", Body: b.String()}
}
