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

// This file holds the module form of the platform draft: a format whose
// platform runs as a whole installation the catalog assembles as one Aggregate
// is drafted as an entry of that Aggregate carrying each source, as [Module]
// describes.

// importModule is importPlatform for a platform carrying a Module: each source
// is drafted as one entry of the module carrying it, every node decided and
// kept in the flow, and what the format says the platform configures for itself
// reported. out holds what importPlatform reported before it was called.
func importModule(in aggrega.ImportInput, served Format, settings importSettings, removal credentialRemoval,
	parse func(syntax string, source []byte) (json.RawMessage, error), out aggrega.ImportOutput) aggrega.ImportOutput {
	p := served.Platform
	m := p.Module
	format := in.Format
	table := newMappingTable(settings.MappingOverrides, format, served)
	catalog := candidatesFrom(in.Catalog)
	platform := catalogEntry(in.Catalog, p.Component)
	reparse := func(source []byte) (json.RawMessage, error) { return parse(served.Syntax, source) }

	sources := sourcesOf(in)
	multiSource := len(sources) > 1

	if settings.AttachEgressAllowlist {
		out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
			Severity: "warn",
			Code:     "feature-not-attached",
			Message: fmt.Sprintf("the attach_egress_allowlist setting attaches %s to the platform a draft carries the "+
				"flow in, and this draft composes %s, whose entry in the drafted Aggregate takes parameters and no "+
				"Feature, so nothing is attached", egressAllowlist, m.Aggregate),
		})
	}

	draft := &platformDraft{settings: settings, format: format, platform: p}
	usedNames := map[string]bool{}
	drafted := false
	kept := fmt.Sprintf("the draft composes %s, which wires every connection of %s itself, so no node of the flow "+
		"is wired", m.Aggregate, p.Component)

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
		inst := newPlatformInstance(UniqueKey(usedNames, path.Base(m.Aggregate)), nil)
		out.Entities = append(out.Entities, aggrega.EmittedEntity{
			Kind: "Aggregate", Name: m.Aggregate, Mode: "reference", SourceRef: src.Name,
		})

		// The nodes are decided in their order, and the flow is carried after:
		// the import reports what carrying raised ahead of what the nodes name.
		var nodeDiags []aggrega.Diagnostic
		for _, node := range nodes {
			ref := refOf(node.ID)
			decision, _ := decideNode(settings, table, catalog, node, ref)
			reason := kept
			if p.Configured != nil {
				if what := p.Configured(tree, node); what != "" {
					reason = fmt.Sprintf("it names %s, and %s configures that for itself; %s", what, p.Component, kept)
					nodeDiags = append(nodeDiags, configuredInPlatform(ref, node, what, p.Component))
				}
			}
			out.Decisions = append(out.Decisions, keptInFlow(decision, p.Component, reason))
		}
		out.Diagnostics = append(out.Diagnostics, inst.carry(p, format, served.Syntax, src, tree, nodes, refOf, nil,
			removal, reparse)...)
		out.Diagnostics = append(out.Diagnostics, nodeDiags...)
		draft.instances = append(draft.instances, inst)
	}

	if drafted && platform == nil {
		out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
			Severity: "warn",
			Code:     "platform-not-in-catalog",
			Message: fmt.Sprintf("the workspace's catalog holds no %s: the draft composes %s, which runs it, all the "+
				"same", p.Component, m.Aggregate),
		})
	}

	out.Entities = append(out.Entities, draft.entity())
	out.Files = render(settings, out.Entities, out.Decisions)
	return out
}

// configuredInPlatform reports a node naming what the platform configures for
// itself: what, the format's noun phrase, and platform, the Component running
// the flow.
func configuredInPlatform(ref string, node Node, what, platform string) aggrega.Diagnostic {
	return aggrega.Diagnostic{
		Severity: "warn",
		Code:     "part-configured-in-platform",
		Message: fmt.Sprintf("node %s (%s) names %s, and %s configures that for itself rather than reaching it "+
			"through a socket, so the draft wires no part for it", ref, node.Type, what, platform),
		SourceRef: ref,
	}
}

// moduleEntity renders the Aggregate of a module-form draft: one entry of the
// module per source, under aggregates, carrying the source in the module's
// parameter, with the comment above it naming what the Release supplies and
// that the import does not check the catalog for the module.
func (d *platformDraft) moduleEntity() aggrega.EmittedEntity {
	p := d.platform
	m := p.Module
	var b strings.Builder
	b.WriteString("apiVersion: aggrega.ai/v1alpha1\n")
	b.WriteString("kind: Aggregate\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: " + d.settings.TargetNamespace + "/flow\n")
	b.WriteString("  version: 0.1.0\n")
	b.WriteString("  description: " + draftDescription(d.format) + "\n")
	b.WriteString("spec:\n")
	b.WriteString("  aggregates:\n")
	for _, inst := range d.instances {
		b.WriteString("    # An import sees the Components of the workspace's catalog and none of its Aggregates, so it " +
			"does not check that the catalog holds " + m.Aggregate + ".\n")
		if len(p.LeftForRelease) > 0 {
			b.WriteString("    # Left for the Release, since the source cannot supply it:\n")
			for _, item := range p.LeftForRelease {
				b.WriteString("    #   - " + item + "\n")
			}
		}
		b.WriteString("    " + inst.key + ":\n")
		b.WriteString("      type: " + m.Aggregate + "\n")
		if inst.carried {
			b.WriteString("      parameters:\n")
			b.WriteString("        " + m.Parameter + ": " + draftedText(inst.definition, 8))
		} else {
			b.WriteString("      # parameters." + m.Parameter + " is left unset: the import's diagnostics say why the " +
				"flow is not carried.\n")
		}
	}
	return aggrega.EmittedEntity{Kind: "Aggregate", Name: d.settings.TargetNamespace + "/flow", Mode: "draft", Body: b.String()}
}
