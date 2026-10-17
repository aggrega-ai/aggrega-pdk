// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
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
// mapping row maps, whose best candidate is at least a MEDIUM match, or which
// Reach names a part, is a part of the flow the platform reaches through a
// socket: it becomes a Component of its own — referenced or drafted exactly as
// the node-by-node draft decides it — wired to the first socket, in the order
// the platform's catalog entry declares its sockets, whose selector the labels
// of its Component satisfy and which Refuse does not refuse it: the referenced
// Component's labels, or, for a drafted one, its best candidate's where that
// candidate is at least a MEDIUM match. Every other node stays inside the flow
// and becomes no Component, and so does a part no socket admits, or whose labels
// nothing supplies, or which Reach refuses, or which Refuse refuses every socket
// its labels satisfy, each of which the import reports. Where the workspace's
// catalog holds no Component of that name the platform is drafted all the same,
// no node becomes a part, and the import says so.
//
// The source is carried as its own bytes, with every credential value it holds
// written empty, every value Wire asks for written anew, every carriage return
// written as a line feed, and nothing else changed, so a source with nothing of
// these to change is carried byte for byte. A source is read as JSON to find
// those values, so a platform's sources are JSON documents.
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
	// format marks as a credential, and every value the format writes empty as
	// one though it marks it as none, as an argument a server is started with can
	// carry a key. Each is written empty before the source is carried, and the
	// import names each one it empties by its node and field, never by its value.
	// Nil locates none.
	Credentials func(tree json.RawMessage) []Credential
	// Match reports whether a set of labels satisfies a socket's selector, in the
	// grammar the catalog writes selectors in. The package carries no selector
	// grammar of its own; nil places no part in any socket.
	Match func(selector string, labels map[string]string) (bool, error)
	// Reach reads, off the source's tree, how a node reaches the part of the flow
	// it stands for, for a format whose nodes may name a part rather than be one,
	// as an MCP Tools node names the MCP server it calls. A node it gives a name
	// is a part of the flow whatever its scores, and the nodes of one source giving
	// one name, placed in one socket, reach one part, drafted once and wired once.
	// A node it refuses is a part no socket takes, and the import reports it. Nil,
	// or a node given neither, leaves the node to its scores, a part of its own.
	Reach func(tree json.RawMessage, node Node) Reach
	// Refuse says, for a node and a socket of the platform whose selector the
	// labels of the node's Component satisfy, why the flow cannot reach the part
	// through a connection of that socket by way of that node, and is empty where
	// it can — for a format whose platform reaches the connections of a socket
	// only through nodes of one kind, as a flow may reach a tools connection only
	// through a node naming it. A socket it refuses is passed over: the part is
	// wired to the next socket whose selector those labels satisfy, and where none
	// remains it stays in the flow, and the import reports it with the refusal.
	// Nil refuses no socket.
	Refuse func(tree json.RawMessage, node Node, socket string) string
	// Wire is asked, for each node wired to a socket, how the flow the platform
	// carries reaches the part: the values of the source it writes anew so that
	// the flow reaches the part through its connection, and what it reports of
	// that. Where a part's Alias differs from the Name Reach gave its node, the
	// edits it returns write the alias in place of that name. Nil writes nothing
	// and reports nothing.
	Wire func(tree json.RawMessage, part Part) Wiring
	// LeftForRelease names the platform's own inputs a source cannot supply, one
	// entry each, which the drafted Aggregate leaves for the Release in a comment
	// above the platform's instance.
	LeftForRelease []string
}

// Reach is how a node reaches the part of the flow it stands for, as
// [Platform.Reach] reads it.
type Reach struct {
	// Name is the name the flow knows the part by. The connection wiring the part
	// to its socket is written under it where a connection's alias can take it —
	// letters, digits, hyphens and underscores — and where no other connection of
	// that socket has an alias differing from it only in letter case or in those
	// hyphens and underscores, since a platform delivering each connection under
	// variables named for its alias would deliver the two under one name.
	// Otherwise the connection is written under an alias derived from the part's
	// key, which [Platform.Wire] is told of through [Part.Alias].
	Name string
	// Refused, where set, says why the node, though a part of the flow, is wired
	// to no socket: it stays in the flow, and the import reports it.
	Refused string
}

// Part is one node of a flow wired to a socket of its platform, as
// [Platform.Wire] is asked about it.
type Part struct {
	Node Node
	// Socket is the socket of the platform the part is wired to.
	Socket string
	// Key is the key the part's Component is written under in the drafted
	// Aggregate, and the target of its connection.
	Key string
	// Name is the name [Platform.Reach] gave the node, empty where it gave none.
	Name string
	// Alias is the alias the part's connection is written under.
	Alias string
}

// Wiring is how the flow a platform carries reaches one of its parts, as
// [Platform.Wire] answers for it.
type Wiring struct {
	// Edits are the values of the source written anew before it is carried, in
	// the form [Rewrite] takes.
	Edits []Edit
	// Reads, where set, says how the carried flow reaches the part through its
	// connection: the import reports it as a warning, and the part's decision
	// carries it.
	Reads string
	// Literal, where set, says why the carried flow reaches the part other than
	// through its connection — at an address the flow holds as written, or one
	// the platform's own defaults give: the import reports it as a warning, and
	// the part's decision carries it.
	Literal string
	// LeftForRelease names what of the part a source cannot supply, one entry
	// each, added to the comment above the platform's instance.
	LeftForRelease []string
}

// aliasShape is the shape a connection's alias takes in an Aggregate.
var aliasShape = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// VariableName is the name a platform delivering a connection under variables
// named for its alias gives that alias: the alias upper-cased, with every
// character other than a letter or a digit written as an underscore. Two
// aliases with one VariableName are not wired to one socket.
func VariableName(alias string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(alias) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
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
		out.Entities = append(out.Entities, aggrega.EmittedEntity{
			Kind: "Component", Name: p.Component, Mode: "reference", SourceRef: src.Name,
		})

		// The nodes are decided first and the flow carried after, since what is
		// carried depends on the parts wired; the import reports what carrying
		// raised ahead of what deciding the nodes raised.
		var nodeDiags []aggrega.Diagnostic
		var wired []wiredNode
		for _, node := range nodes {
			ref := refOf(node.ID)
			decision, entity := decideNode(settings, table, catalog, node, ref)
			var reach Reach
			if p.Reach != nil {
				reach = p.Reach(tree, node)
			}
			named := reach.Name != "" || reach.Refused != ""
			if !named && !isPart(table, node, decision) {
				out.Decisions = append(out.Decisions, keptInFlow(decision, p.Component, notAPart(decision)))
				continue
			}
			if reach.Refused != "" {
				out.Decisions = append(out.Decisions, keptInFlow(decision, p.Component, reach.Refused))
				nodeDiags = append(nodeDiags, partNotPlaced(ref, node, p.Component, reach.Refused))
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
				nodeDiags = append(nodeDiags, partNotPlaced(ref, node, p.Component, reason))
				continue
			}
			var refuse func(socket string) string
			if p.Refuse != nil {
				refuse = func(socket string) string { return p.Refuse(tree, node, socket) }
			}
			socket, refusals, passed := placePart(platform, labels[target], target != "", p.Match, refuse)
			for _, r := range refusals {
				if !unreadable[r.socket] {
					unreadable[r.socket] = true
					nodeDiags = append(nodeDiags, aggrega.Diagnostic{
						Severity: "warn",
						Code:     "socket-selector-unreadable",
						Message: fmt.Sprintf("the selector of the %s socket %s declares does not read, so no part is wired to it: %s",
							r.socket, p.Component, r.err),
					})
				}
			}
			if socket == "" {
				reason := unplacedReason(target, p.Component, p.Match != nil, passed)
				out.Decisions = append(out.Decisions, keptInFlow(decision, p.Component, reason))
				nodeDiags = append(nodeDiags, partNotPlaced(ref, node, p.Component, reason))
				continue
			}

			if reach.Name != "" {
				if shared, ok := inst.named[namedPart{socket, reach.Name}]; ok {
					decision.Rationale += fmt.Sprintf("; wired to the %s socket of %s as %s, through the connection %s, "+
						"which node %s reaches too", socket, p.Component, shared.key, shared.alias, shared.ref)
					decision.Rationale += passedOverClause(passed)
					wired = append(wired, wiredNode{decision: len(out.Decisions), ref: ref,
						part: Part{Node: node, Socket: socket, Key: shared.key, Name: reach.Name, Alias: shared.alias}})
					out.Decisions = append(out.Decisions, decision)
					continue
				}
			}
			key := uniqueKey(usedNames, kebab(firstOf(reach.Name, node.ID)))
			alias := inst.aliasFor(socket, reach.Name, key)
			decision.Rationale += fmt.Sprintf("; wired to the %s socket of %s as %s", socket, p.Component, key)
			if alias != key {
				decision.Rationale += ", through the connection " + alias
			}
			if reach.Name != "" && alias != reach.Name {
				decision.Rationale += fmt.Sprintf("; the flow names it %q, which cannot name its connection, so it is "+
					"named %s in the flow the draft carries", reach.Name, alias)
			}
			decision.Rationale += passedOverClause(passed)
			if reach.Name != "" {
				inst.named[namedPart{socket, reach.Name}] = sharedPart{key: key, alias: alias, ref: ref}
			}
			wired = append(wired, wiredNode{decision: len(out.Decisions), ref: ref,
				part: Part{Node: node, Socket: socket, Key: key, Name: reach.Name, Alias: alias}})
			out.Decisions = append(out.Decisions, decision)
			out.Entities = append(out.Entities, entity)
			inst.connect(socket, alias, key)
			inst.parts = append(inst.parts, platformPart{key: key, decision: decision, entity: entity})
		}

		var edits []Edit
		if p.Wire != nil {
			for i := range wired {
				wired[i].wiring = p.Wire(tree, wired[i].part)
				edits = append(edits, wired[i].wiring.Edits...)
			}
		}
		out.Diagnostics = append(out.Diagnostics, inst.carry(p, format, src, tree, nodes, refOf, edits)...)
		out.Diagnostics = append(out.Diagnostics, nodeDiags...)
		for _, w := range wired {
			inst.leave(w.wiring.LeftForRelease...)
			if !inst.carried {
				// The flow is not carried, so nothing reaches the part through it;
				// the import has said why.
				continue
			}
			d := &out.Decisions[w.decision]
			part := w.part
			if w.wiring.Reads != "" {
				d.Rationale += "; the flow reaches it through that connection: " + w.wiring.Reads
				out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
					Severity: "warn",
					Code:     "part-reads-connection",
					Message: fmt.Sprintf("node %s reaches %s through the %s connection %s of %s: %s", w.ref, part.Key,
						part.Socket, part.Alias, p.Component, w.wiring.Reads),
					SourceRef: w.ref,
				})
			}
			if w.wiring.Literal != "" {
				d.Rationale += "; the flow reaches it other than through that connection: " + w.wiring.Literal
				out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
					Severity: "warn",
					Code:     "part-address-in-flow",
					Message: fmt.Sprintf("node %s reaches %s, wired to the %s socket of %s as %s, other than through that "+
						"connection: %s", w.ref, part.Key, part.Socket, p.Component, part.Alias, w.wiring.Literal),
					SourceRef: w.ref,
				})
			}
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

// wiredNode is one node wired to a socket of the platform, awaiting what the
// format says of how the flow reaches its part: the position of its decision,
// its reference, the part, and the format's answer.
type wiredNode struct {
	decision int
	ref      string
	part     Part
	wiring   Wiring
}

// namedPart identifies a part nodes reach by name: the socket it is wired to and
// the name.
type namedPart struct{ socket, name string }

// sharedPart is a part nodes reach by name: its key, its connection's alias, and
// the reference of the node that first reached it.
type sharedPart struct{ key, alias, ref string }

// firstOf returns the first of its arguments that is not empty.
func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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

// passedOver is one socket whose selector a part's labels satisfy and which the
// format refused the part, with why.
type passedOver struct{ socket, why string }

// placePart returns the first socket of the platform, in the order its catalog
// entry declares them, whose selector the labels satisfy and which refuse — nil
// for none — does not refuse, or "" where none does; with every socket whose
// selector the matcher refused to read, and every socket passed over because
// refuse refused it. A part with no Component to read labels off, and any part
// where there is no matcher, is placed nowhere.
func placePart(platform *aggrega.CatalogComponent, labels map[string]string, known bool,
	match func(selector string, labels map[string]string) (bool, error),
	refuse func(socket string) string) (string, []socketRefusal, []passedOver) {
	if match == nil || !known {
		return "", nil, nil
	}
	var refusals []socketRefusal
	var passed []passedOver
	for _, s := range platform.Sockets {
		ok, err := match(s.Selector, labels)
		if err != nil {
			refusals = append(refusals, socketRefusal{socket: s.Name, err: err})
			continue
		}
		if !ok {
			continue
		}
		if refuse != nil {
			if why := refuse(s.Name); why != "" {
				passed = append(passed, passedOver{socket: s.Name, why: why})
				continue
			}
		}
		return s.Name, refusals, passed
	}
	return "", refusals, passed
}

// unplacedReason says why a part was placed in no socket of the platform: passed
// are the sockets whose selector its Component satisfies and which the format
// refused it.
func unplacedReason(target, platform string, matching bool, passed []passedOver) string {
	switch {
	case target == "":
		return "a mapping row maps its type to no Component the workspace's catalog holds, so no socket of " + platform +
			" can take it"
	case !matching:
		return "it maps to " + target + ", but this importer reads no socket selector, so no socket of " + platform +
			" takes it"
	case len(passed) > 0:
		clauses := make([]string, 0, len(passed))
		for _, p := range passed {
			clauses = append(clauses, fmt.Sprintf("the selector of the %s socket of %s, but %s", p.socket, platform, p.why))
		}
		return "it maps to " + target + ", which satisfies " + strings.Join(clauses, ", and ") +
			"; no other socket of " + platform + " takes it"
	default:
		return "it maps to " + target + ", which satisfies the selector of no socket " + platform + " declares"
	}
}

// passedOverClause says, in a wired part's decision, which sockets its Component
// satisfies were passed over before the one it is wired to, and why; empty where
// none was.
func passedOverClause(passed []passedOver) string {
	var b strings.Builder
	for _, p := range passed {
		fmt.Fprintf(&b, "; not to the %s socket, whose selector its Component satisfies too, since %s", p.socket, p.why)
	}
	return b.String()
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
	uses    map[string][]connection
	parts   []platformPart
	// named are the parts nodes reach by name, and delivered the VariableName of
	// every alias each socket's connections are written under.
	named     map[namedPart]sharedPart
	delivered map[string]map[string]bool
	// left is what of the parts the Release supplies, in the order the parts were
	// wired.
	left []string
}

// connection is one connection a socket of the platform draws: its alias and the
// key of the part it reaches.
type connection struct{ alias, target string }

// platformPart is one part of the flow drafted as a Component of its own.
type platformPart struct {
	key      string
	decision aggrega.ImportDecision
	entity   aggrega.EmittedEntity
}

// newPlatformInstance returns an instance keyed key of the platform whose
// catalog entry is entry, nil where the catalog holds none.
func newPlatformInstance(key string, entry *aggrega.CatalogComponent) *platformInstance {
	inst := &platformInstance{key: key, uses: map[string][]connection{}, named: map[namedPart]sharedPart{},
		delivered: map[string]map[string]bool{}}
	if entry != nil {
		for _, s := range entry.Sockets {
			inst.sockets = append(inst.sockets, s.Name)
		}
	}
	return inst
}

// aliasFor returns the alias the connection of socket to the part keyed key is
// written under, and records it taken: name where [Reach] gave one an alias can
// take, otherwise key, or key with a numeric suffix, so no two connections of the
// socket share a VariableName.
func (inst *platformInstance) aliasFor(socket, name, key string) string {
	taken := inst.delivered[socket]
	if taken == nil {
		taken = map[string]bool{}
		inst.delivered[socket] = taken
	}
	alias := name
	if alias == "" || !aliasShape.MatchString(alias) || taken[VariableName(alias)] {
		alias = key
		for i := 2; !aliasShape.MatchString(alias) || taken[VariableName(alias)]; i++ {
			alias = fmt.Sprintf("%s-%d", key, i)
		}
	}
	taken[VariableName(alias)] = true
	return alias
}

// connect records a connection of socket, written under alias, to the part keyed
// key.
func (inst *platformInstance) connect(socket, alias, key string) {
	inst.uses[socket] = append(inst.uses[socket], connection{alias: alias, target: key})
}

// leave records what of a part the Release supplies, each entry once.
func (inst *platformInstance) leave(items ...string) {
	for _, item := range items {
		seen := false
		for _, have := range inst.left {
			seen = seen || have == item
		}
		if !seen {
			inst.left = append(inst.left, item)
		}
	}
}

// wired reports whether any socket of the instance draws a connection.
func (inst *platformInstance) wired() bool {
	return len(inst.parts) > 0
}

// carry prepares the flow one source carries into the instance: the values the
// format writes anew written, its credential values written empty, its carriage
// returns written as line feeds, its text escaped for the Aggregate's template
// reading, and its identifier. It returns the diagnostics the preparation
// raised: one per credential value it emptied, naming the node and the field;
// one naming how many carriage returns it wrote as line feeds; and, where the
// flow cannot be carried, why — a value to write or empty that the source's bytes
// do not hold where its tree does, or a character a YAML literal block cannot
// carry as written. Where the source gives its flow no identifier a draft can
// carry, one is derived from the carried bytes — or, where a value could not be
// located and the flow is not carried, from its nodes' identifiers, so nothing a
// credential value could reach decides it.
func (inst *platformInstance) carry(p *Platform, format string, src aggrega.ImportSource, tree json.RawMessage,
	nodes []Node, refOf func(string) string, edits []Edit) []aggrega.Diagnostic {
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
	scrubbed, emptied, err := rewriteSource(src.Bytes, edits, creds)
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
			Severity:  "warn",
			Code:      "credential-emptied",
			Message:   emptiedMessage(c, refOf(c.Node)),
			SourceRef: refOf(c.Node),
		})
	}

	text, returns := lineFeedsFor(string(scrubbed))
	if returns > 0 {
		diags = append(diags, aggrega.Diagnostic{
			Severity: "warn",
			Code:     "carriage-returns-rewritten",
			Message: fmt.Sprintf("%s holds %d carriage return(s), each written as a line feed in values.%s, since a YAML "+
				"literal block cannot carry one: a JSON reader reads the carried flow as the same value", flow, returns,
				p.DefinitionValue),
			SourceRef: src.Name,
		})
	}
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
		inst.identifier = derivedFlowID(format, []byte(text))
		inst.idNote = "derived from the exported flow, which gives no identifier of its own"
	default:
		inst.identifier = derivedFlowID(format, []byte(text))
		inst.idNote = "derived from the exported flow: its own identifier holds characters a draft does not write as they are"
	}
	return diags
}

// emptiedMessage reports a credential value written empty, by its node — written
// ref — and its field, never by its value: with the credential's Value set, the
// parts inside the value its Detail names; with only its Detail set, the whole
// value, and why.
func emptiedMessage(c Credential, ref string) string {
	const carried = " in the flow the draft carries"
	const none = ": no credential from the source reaches the draft"
	switch {
	case c.Value != nil:
		return fmt.Sprintf("%s of node %s is written with its %s empty%s%s", c.Field, ref,
			firstOf(c.Detail, "credential values"), carried, none)
	case c.Detail != "":
		return fmt.Sprintf("the whole value of %s of node %s is written empty%s, since %s%s", c.Field, ref, carried,
			c.Detail, none)
	default:
		return fmt.Sprintf("the value of %s, which node %s marks as a credential, is written empty%s%s", c.Field, ref,
			carried, none)
	}
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
		if len(p.LeftForRelease) > 0 || len(inst.left) > 0 || inst.wired() {
			b.WriteString("    # Left for the Release, since the source cannot supply it:\n")
			for _, item := range p.LeftForRelease {
				b.WriteString("    #   - " + item + "\n")
			}
			for _, item := range inst.left {
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
				for _, c := range inst.uses[socket] {
					b.WriteString("          " + c.alias + ":\n")
					b.WriteString("            target: " + c.target + "\n")
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
