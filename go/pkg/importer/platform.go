// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
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
// its labels satisfy, or which is placed in a socket of Single whose one
// connection another part holds, each of which the import reports. Where the
// workspace's catalog holds no Component of that name the platform is drafted
// all the same, no node becomes a part, and the import says so. A referenced
// part whose Component declares a value named model is written with the model
// its node selects there, as Model reads it, or, where its node names none,
// with a comment leaving it for the author, which the import reports. Where the
// attach_egress_allowlist setting is true, each instance also carries
// core/egress-allowlist under features, with nothing of its infrastructure set,
// and the drafted Release's entry for the instance names what of that the
// Release sets.
//
// The source is carried as its own bytes, with every value Wire and Reached ask
// for written anew, every credential value Credentials locates written empty
// where the empty_credentials setting asks for it — and every other one carried
// as written and named in a warning — every carriage return written as a line
// feed, and nothing else changed, so a source with nothing of these to change is
// carried byte for byte. A source is read as JSON to find those values, unless
// EmptyInSource writes them, so a platform's sources are JSON documents where it
// is nil.
//
// Where Module is set, the platform runs as a whole installation the catalog
// assembles as one Aggregate, and each source is drafted in the module form
// instead: one entry of that Aggregate per source, carrying the source in its
// parameter, with no part of the flow wired, as [Module] describes.
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
	// Credentials locates every value the source's tree holds that can carry a
	// credential, of every class, each tagged with its class: what the format
	// marks as a credential, each member of an MCP server's configuration, and
	// each input the format marks as no credential that can carry one, as a
	// request's headers or query parameters can. It reads no setting: the
	// empty_credentials setting decides which classes are written empty before
	// the source is carried, and every other value located is carried as
	// written; the import names each value it empties and each it carries
	// holding something by its node and field, never by its value. A value Wire
	// or Reached writes is theirs, neither emptied nor carried, and a value that
	// is wholly a variable reference ([IsVariableReference]) is left as written,
	// named in no report, so the locator returns it as any other. Nil locates
	// none.
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
	// Reached is asked, for each node of a source the draft wires to no socket,
	// whether a connection the draft does wire reaches the node as it is written —
	// its client reading what the platform delivers for that connection from its
	// environment, as the format reads it off the tree — and, where one does, what
	// the flow is written with so that nothing the node holds is sent in place of
	// what that connection supplies: the values of the source it writes anew, and
	// how the node reaches the connection, which the import reports. wired are the
	// parts of the platform's instance the draft wires, in the order wired. A
	// Wiring with no Reads says the node reaches none, and its edits are not
	// written. Nil reaches no node.
	Reached func(tree json.RawMessage, node Node, wired []Part) Wiring
	// Single lists the sockets of the platform that draw a single connection, as
	// a socket does whose connection the platform's Recipes deliver under fixed
	// variable names, which can carry one connection only. Of the parts placed
	// in such a socket one is wired, as [SingleSocket] chooses it; every other
	// stays in the flow, and the import reports it with the part that holds the
	// connection and, as Standing says, how the flow reaches it as its node is
	// written. Nil lists none: every socket draws a connection per part.
	Single []SingleSocket
	// Standing says, for a node whose part stays in the flow because another
	// part holds the single connection of the socket it was placed in, whether
	// the flow reaches the node's part through that connection as the node is
	// written, nothing written anew, and how — or how it reaches it instead.
	// part is the node as it would be wired: in that socket, under the key of
	// the part holding the connection and the connection's alias. An empty how
	// says nothing. Nil says nothing of any such node.
	Standing func(tree json.RawMessage, part Part) (through bool, how string)
	// Model reads, off the source's tree, the name of the model a node selects:
	// the model the flow asks the part it stands for to serve when it runs. It is
	// asked about each part wired to a socket whose Component the draft
	// references and declares a value named model, which takes that name: the
	// draft writes the name it gives into the part's values.model. Where it gives
	// none, why completes "the node …" with why the node names no model a draft
	// can write — that it selects none, or names it through something read only
	// when the flow runs — and is empty where the format does not read a model
	// off a node of that type; the draft leaves values.model for the author with
	// a comment saying so, and the import reports it. why is written into that
	// comment, so it carries nothing read from the source. A drafted part is not
	// asked. Nil reads no model, and the draft writes nothing of one.
	Model func(tree json.RawMessage, node Node) (name, why string)
	// LeftForRelease names the platform's own inputs a source cannot supply, one
	// item each, which the drafted Release's entry for the platform's instance
	// leaves for the Release, as [ReleaseEntry.Left] writes them: each item's path
	// is relative to that entry — the instance's own entry, or, in the module form,
	// the routing entry of the module.
	LeftForRelease []LeftItem
	// TargetAttachments are attachments of the platform's own instance that the
	// Recipes realizing the platform on one target need declared in the
	// Aggregate, and those of the other targets do not. Each is written only
	// where the instance draws a connection, and by the target the import drafts
	// for: one whose Target is that target is declared, under a comment saying
	// why; one of another target is left out; and where the import names no
	// target, a comment inside the instance shows, for each, the lines an author
	// uncomments for its target.
	TargetAttachments []TargetAttachment
	// Module, where set, is the catalog Aggregate the platform runs as a whole
	// installation in, Component among the parts it composes and wires: each source
	// is drafted as an entry of it rather than as an instance of Component, as
	// [Module] describes. Nil drafts an instance of Component.
	Module *Module
	// Configured says, for a node of a source drafted in the module form, what the
	// node names that the platform configures for itself rather than reaching
	// through a socket — a model and the provider serving it, a tool's provider, a
	// knowledge base, a plugin — as a noun phrase completing "node … names", empty
	// where it names nothing of the kind. The import reports each in the node's
	// decision and in a warning, so it carries only what the format reads off the
	// node, and never a credential value. Nil reports none.
	Configured func(tree json.RawMessage, node Node) string
	// EmptyInSource writes empty, in the source's own bytes, each value of
	// credentials, for a format whose sources are not JSON documents: each value
	// addressed by its pointer is written as the empty string — or as its Value,
	// where that is set — and every other byte is kept. The import asks it only
	// where the empty_credentials setting writes values empty, and only about the
	// values whose tree holds something to write; it re-reads what it returns
	// through the host's parser, and carries the result only where that reads back
	// as the source's tree with exactly those values written so. Otherwise it
	// carries nothing of the source, and says why. A source the setting leaves as
	// written is never handed to it, and is carried byte for byte. An error it
	// returns is reported, so it names no value. Nil locates the values in the
	// source's bytes read as JSON.
	EmptyInSource func(source []byte, credentials []Credential) ([]byte, error)
}

// Module is the catalog Aggregate a platform running as a whole installation is
// drafted as — the module form of the platform draft. The drafted Aggregate
// composes one entry of it per source, keyed by the last segment of its name,
// whose parameter named Parameter carries the source, written as the platform
// draft writes a flow: as its own bytes, with only what the empty_credentials
// setting writes empty changed. The module wires every connection of the
// platform itself, so no node of the flow is wired to a socket and no part is
// drafted; each node stays in the flow, and what [Platform.Configured] says the
// platform configures for itself is reported. No identifier is written, the
// platform's LeftForRelease is left on the drafted Release's routing entry for
// the module, and a comment above the Aggregate's entry says that the catalog the
// import is handed lists Components only, so the import does not check that the
// workspace's catalog holds Aggregate. An entry of a
// composed Aggregate takes parameters and no Feature, so the
// attach_egress_allowlist setting attaches nothing to it, and the import says so.
// The parameters of Aggregate a Release supplies are declared by the drafted
// Aggregate and passed down to every entry, as PassedDown says.
type Module struct {
	// Aggregate is the name of the catalog Aggregate, such as core/dify.
	Aggregate string
	// Parameter is the parameter of Aggregate that carries the flow.
	Parameter string
	// PassedDown lists parameters of Aggregate that no source supplies and that
	// Aggregate gives no default, such as a key the installation signs with. The
	// drafted Aggregate declares each once, under spec.parameters, as a string
	// parameter of its own, and passes it down by the same name to every entry,
	// as [PassedDownParameter] describes, so the Release fulfills it and a build
	// whose Release leaves it unset halts naming it. Every entry of one draft is
	// handed the one parameter. The drafted Release's entry for the module leaves
	// each for the Release. Empty passes nothing down, and the drafted Aggregate declares
	// no parameter.
	PassedDown []PassedDownParameter
}

// PassedDownParameter is a parameter of a module that the drafted Aggregate
// declares and passes down to every entry of the module, as [Module] lists them.
// The drafted Aggregate declares it as a string parameter — required, at least
// one character long, with no default and with Description as its description —
// and hands each entry NAME: "{{ .parameters.NAME }}".
type PassedDownParameter struct {
	// Name is the parameter's name, the module's and the drafted Aggregate's
	// alike. It is written as it is, both as a YAML key and as a field of the
	// template the compiler evaluates the Aggregate's text as, so the caller gives
	// an identifier — a letter or an underscore, then letters, digits and
	// underscores — that is neither the module's Parameter nor the Name of any
	// other parameter the module passes down. The import checks none of this.
	Name string
	// Description is the declared parameter's description. It is written through
	// EscapeTemplateText and then as a double-quoted scalar by QuotedString, so it
	// reads back as written, whatever UTF-8 text it holds.
	Description string
}

// TargetAttachment is an attachment of a platform's own instance that its
// Recipes on one target need declared in the Aggregate, as [Platform] lists
// them: a secret the Release attaches, declared with the key its Recipes read it
// under, as a target whose Recipes read every secret the Release alone attaches
// under one default key needs, so that the secret and a connection's credential
// do not arrive under one key.
type TargetAttachment struct {
	// Target is the target, as the Recipes realizing the platform on it are
	// named, such as compose.
	Target string
	// Slot is the platform's attachment slot, Type the type of the attachment
	// declared there, and Key the key its values.key names.
	Slot, Type, Key string
	// Why completes the comment saying why the target needs the lines. It is
	// written into the draft, so it carries nothing read from the source.
	Why string
}

// modelValue is the value a Component serving a model takes that model's name
// in, as every model Component of the standard library declares it.
const modelValue = "model"

// noModelField is why a node names no model the draft can write where the
// format does not read a model off a node of its type.
const noModelField = "is of a type whose field naming a model this importer does not read"

// SingleSocket is a socket of a platform that draws a single connection, as
// [Platform.Single] lists them, and how the part its connection goes to is
// chosen where several parts are placed in it: the first part, in the order the
// source lists its nodes, whose Component carries every label of Prefer, and,
// where none does, the first part. A part's Component is the one placing it
// reads labels off: the referenced Component, or a drafted part's best
// candidate.
type SingleSocket struct {
	// Name is the socket's name, as the platform's catalog entry declares it.
	Name string
	// Prefer are the labels marking the Component of the part the connection
	// goes to where several parts are placed in the socket. Empty prefers none,
	// and the first part holds it.
	Prefer map[string]string
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
	// LeftForRelease names what of the part a source cannot supply, one item each,
	// which the drafted Release's entry for the part leaves for the Release, each
	// item's path relative to that entry.
	LeftForRelease []LeftItem
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
	removal, halt := readCredentialRemoval(in.Settings)
	if halt != nil {
		return aggrega.ImportOutput{Diagnostics: []aggrega.Diagnostic{*halt}}
	}
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
	reparse := func(source []byte) (json.RawMessage, error) { return parse(served.Syntax, source) }
	if p.Module != nil {
		return importModule(in, served, settings, removal, parse, out)
	}

	draft := &platformDraft{settings: settings, format: format, platform: p}
	usedNames := map[string]bool{}
	unreadable := map[string]bool{}
	drafted := false

	for _, src := range sources {
		tree, err := parse(served.Syntax, src.Bytes)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, parseFailed(Named(in, src), format, err))
			continue
		}
		nodes, _ := walk(served, format, tree)
		if len(nodes) == 0 {
			out.Diagnostics = append(out.Diagnostics, noNodes(Named(in, src), format))
			continue
		}
		drafted = true
		draft.titles = append(draft.titles, titleOf(served, format, tree))

		refOf := func(id string) string {
			if multiSource {
				return src.Name + "#" + id
			}
			return id
		}
		inst := newPlatformInstance(UniqueKey(usedNames, path.Base(p.Component)), platform)
		out.Entities = append(out.Entities, aggrega.EmittedEntity{
			Kind: "Component", Name: p.Component, Mode: "reference", SourceRef: src.Name,
		})

		// The nodes are placed first, every one of them, since which part a
		// socket drawing a single connection goes to depends on all the parts
		// placed in it; then wired, in the order of the nodes; and the flow is
		// carried after, since what is carried depends on the parts wired. The
		// import reports what carrying raised ahead of what deciding the nodes
		// raised, and what deciding them raised in the order of the nodes.
		placements := make([]placement, 0, len(nodes))
		for _, node := range nodes {
			placements = append(placements, placeNode(p, platform, settings, table, catalog, labels, unreadable, tree,
				node, refOf(node.ID)))
		}
		holders := holdSingleSockets(p, placements, labels)

		var nodeDiags []aggrega.Diagnostic
		var wired []wiredNode
		var held []heldOut
		for i := range placements {
			pl := &placements[i]
			node, ref, decision, entity, reach, socket, passed := pl.node, pl.ref, pl.decision, pl.entity, pl.reach,
				pl.socket, pl.passed
			nodeDiags = append(nodeDiags, pl.diags...)
			if pl.heldBy != "" {
				held = append(held, heldOut{placement: i, decision: len(out.Decisions), diag: len(nodeDiags)})
				pl.decisionAt = len(out.Decisions)
				out.Decisions = append(out.Decisions, decision)
				// Written once the part holding the connection is wired and the
				// flow carried, since it names both.
				nodeDiags = append(nodeDiags, aggrega.Diagnostic{})
				continue
			}
			if socket == "" {
				pl.decisionAt = len(out.Decisions)
				out.Decisions = append(out.Decisions, decision)
				continue
			}
			holder, holds := holders[socket]
			holds = holds && holder.placement == i

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
			key := UniqueKey(usedNames, firstOf(reach.Name, node.ID))
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
			if holds {
				holder.key, holder.alias = key, alias
				decision.Rationale += fmt.Sprintf("; the %s socket of %s draws a single connection, which this part "+
					"holds since %s", socket, p.Component, holder.why)
			}
			if reach.Name != "" {
				inst.named[namedPart{socket, reach.Name}] = sharedPart{key: key, alias: alias, ref: ref}
			}
			part := platformPart{key: key, entity: entity}
			if asked, model, why := partModel(p, in.Catalog, tree, node, entity); asked {
				if model != "" {
					part.model = model
					decision.Rationale += fmt.Sprintf("; values.%s is written as %q, the model the node selects", modelValue,
						model)
				} else {
					part.modelLeft = why
					decision.Rationale += fmt.Sprintf("; values.%s is left for the author, since the node %s", modelValue, why)
					nodeDiags = append(nodeDiags, modelNotNamed(ref, node, key, entity.Name, why))
				}
			}
			part.decision = decision
			wired = append(wired, wiredNode{decision: len(out.Decisions), ref: ref,
				part: Part{Node: node, Socket: socket, Key: key, Name: reach.Name, Alias: alias}})
			out.Decisions = append(out.Decisions, decision)
			out.Entities = append(out.Entities, entity)
			inst.connect(socket, alias, key)
			inst.parts = append(inst.parts, part)
		}

		var edits []Edit
		if p.Wire != nil {
			for i := range wired {
				wired[i].wiring = p.Wire(tree, wired[i].part)
				edits = append(edits, wired[i].wiring.Edits...)
			}
		}
		// A node the draft wires to no socket that a connection reaches as it is
		// written is written so that nothing it holds is sent in place of what
		// that connection supplies, before the flow is carried.
		var reached []reachedNode
		if p.Reached != nil && len(wired) > 0 {
			parts := make([]Part, 0, len(wired))
			for _, w := range wired {
				parts = append(parts, w.part)
			}
			for i := range placements {
				pl := &placements[i]
				if pl.socket != "" && pl.heldBy == "" {
					continue
				}
				w := p.Reached(tree, pl.node, parts)
				if w.Reads == "" {
					continue
				}
				reached = append(reached, reachedNode{placement: i, wiring: w})
				edits = append(edits, w.Edits...)
			}
		}
		out.Diagnostics = append(out.Diagnostics, inst.carry(p, format, served.Syntax, src, tree, nodes, refOf, edits,
			removal, reparse)...)
		for _, h := range held {
			pl := placements[h.placement]
			reason := heldOutReason(p, inst.carried, tree, pl, holders[pl.heldBy])
			out.Decisions[h.decision] = keptInFlow(out.Decisions[h.decision], p.Component, reason)
			nodeDiags[h.diag] = partNotPlaced(pl.ref, pl.node, p.Component, reason)
		}
		out.Diagnostics = append(out.Diagnostics, nodeDiags...)
		for _, w := range wired {
			inst.leave(w.part.Key, w.wiring.LeftForRelease...)
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
		for _, r := range reached {
			if !inst.carried {
				continue
			}
			pl := placements[r.placement]
			out.Decisions[pl.decisionAt].Rationale += "; a connection the draft wires reaches it as it is written: " +
				r.wiring.Reads
			out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
				Severity: "warn",
				Code:     "part-reads-connection",
				Message: fmt.Sprintf("node %s is wired to no socket of %s, and a connection the draft wires reaches it as "+
					"it is written: %s", pl.ref, p.Component, r.wiring.Reads),
				SourceRef: pl.ref,
			})
		}
		if settings.AttachEgressAllowlist {
			for _, w := range wired {
				out.Decisions[w.decision].Rationale += fmt.Sprintf("; the %s attachment of %s admits its connection to %s",
					egressAllowlistKey, inst.key, w.part.Key)
			}
			out.Diagnostics = append(out.Diagnostics, egressAllowlistAttached(inst.key, src.Name))
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

	out.Entities = append(out.Entities, draft.entity(in), draft.release(in))
	out.Files = render(settings, out.Entities, out.Decisions)
	return out
}

// release drafts the Release deploying the drafted Aggregate, through
// [DraftRelease]: an entry per instance, in the order the Aggregate declares them
// — each platform instance, followed by the parts wired to it — since a build
// places only the instances a Release names, each leaving for the Release what
// no source supplies of it. In the module form each entry is a module of the
// Aggregate, routed under the boundary the platform runs in, named platform.
func (d *platformDraft) release(in aggrega.ImportInput) aggrega.EmittedEntity {
	var entries []ReleaseEntry
	for _, inst := range d.instances {
		entries = append(entries, ReleaseEntry{Key: inst.key, Module: d.platform.Module != nil,
			Left: d.instanceLeft(inst)})
		for _, part := range inst.parts {
			entries = append(entries, ReleaseEntry{Key: part.key, Left: inst.left[part.key]})
		}
	}
	return DraftRelease(in, ReleaseDraft{Boundary: platformBoundary, Entries: entries}, d.titles...)
}

// instanceLeft is what the Release's entry for a platform instance leaves for
// the Release: in the module form, each parameter the module's PassedDown lists,
// then the platform's own LeftForRelease; otherwise the platform's own
// LeftForRelease, the credential the instance presents on each connection it
// draws, in the order its sockets and their connections are written, and, where
// the attach_egress_allowlist setting is true, what of that attachment the
// Release sets.
func (d *platformDraft) instanceLeft(inst *platformInstance) []LeftItem {
	p := d.platform
	var left []LeftItem
	if m := p.Module; m != nil {
		for _, param := range m.PassedDown {
			left = append(left, LeftItem{Text: "the " + param.Name + " parameter, under the Release's spec.parameters, " +
				"which the drafted Aggregate declares and passes down to " + m.Aggregate})
		}
		return append(left, p.LeftForRelease...)
	}
	left = append(left, p.LeftForRelease...)
	for _, socket := range inst.sockets {
		for _, c := range inst.uses[socket] {
			left = append(left, ConnectionCredential(inst.key, socket, c.alias, c.target))
		}
	}
	if d.settings.AttachEgressAllowlist {
		left = append(left,
			LeftItem{Path: "features." + egressAllowlistKey + "." + egressAffirmation, Text: "the affirmation that the " +
				"cluster's network plugin enforces network policy, without which the build halts"},
			LeftItem{Path: "features." + egressAllowlistKey + "." + egressRanges, Text: "an address range for each " +
				"connection whose target the cluster does not run, such as a hosted model, without which the build halts " +
				"naming that connection"},
			LeftItem{Text: "aggrega.ai/namespace on the boundary holding " + inst.key + ", where the target emits its " +
				"workloads into no namespace: the attachment's network policy is placed in its workload's namespace, and " +
				"the build halts without one"})
	}
	return left
}

// platformBoundary is the boundary a drafted Release places a module-form
// platform in: a Release places a module only under a boundary.
const platformBoundary = "platform"

// wiredNode is one node wired to a socket of the platform, awaiting what the
// format says of how the flow reaches its part: the position of its decision,
// its reference, the part, and the format's answer.
type wiredNode struct {
	decision int
	ref      string
	part     Part
	wiring   Wiring
}

// placement is what placing one node decided, before any part is wired: its
// decision and the entity it would be emitted as, how it reaches its part, the
// Component whose labels placed it, and the socket it is wired to and the
// sockets passed over for it — or, with no socket, the decision already says
// why the flow keeps it. diags are what placing it raised.
type placement struct {
	node     Node
	ref      string
	decision aggrega.ImportDecision
	entity   aggrega.EmittedEntity
	reach    Reach
	target   string
	socket   string
	passed   []passedOver
	diags    []aggrega.Diagnostic
	// heldBy, where set, is the socket drawing a single connection the node's
	// part was placed in, which another part holds: the part stays in the flow.
	heldBy string
	// decisionAt is the position of the decision of a node the draft wires to no
	// socket, once it is recorded.
	decisionAt int
}

// reachedNode is a node the draft wires to no socket that a connection the
// draft wires reaches as it is written: its placement, and what the format says
// the flow is written with for it.
type reachedNode struct {
	placement int
	wiring    Wiring
}

// placeNode decides one node of a source and places it: a part of the flow in
// the socket of the platform it is wired to, or a node the flow keeps, with
// why. unreadable records the sockets whose unreadable selector the import has
// reported already, so each is reported once.
func placeNode(p *Platform, platform *aggrega.CatalogComponent, settings importSettings, table mappingTable,
	catalog []candidate, labels map[string]map[string]string, unreadable map[string]bool, tree json.RawMessage,
	node Node, ref string) placement {
	pl := placement{node: node, ref: ref}
	pl.decision, pl.entity = decideNode(settings, table, catalog, node, ref)
	if p.Reach != nil {
		pl.reach = p.Reach(tree, node)
	}
	keep := func(reason string, report bool) placement {
		pl.decision = keptInFlow(pl.decision, p.Component, reason)
		if report {
			pl.diags = append(pl.diags, partNotPlaced(ref, node, p.Component, reason))
		}
		return pl
	}
	named := pl.reach.Name != "" || pl.reach.Refused != ""
	if !named && !isPart(table, node, pl.decision) {
		return keep(notAPart(pl.decision), false)
	}
	if pl.reach.Refused != "" {
		return keep(pl.reach.Refused, true)
	}
	pl.target = partTarget(pl.decision)
	if platform == nil {
		reason := fmt.Sprintf("it maps to %s, but the workspace's catalog holds no %s, so no socket of it can be read",
			pl.target, p.Component)
		if pl.target == "" {
			reason = fmt.Sprintf("a mapping row maps its type, but the workspace's catalog holds no %s, so no socket "+
				"of it can be read", p.Component)
		}
		return keep(reason, true)
	}
	var refuse func(socket string) string
	if p.Refuse != nil {
		refuse = func(socket string) string { return p.Refuse(tree, node, socket) }
	}
	socket, refusals, passed := placePart(platform, labels[pl.target], pl.target != "", p.Match, refuse)
	for _, r := range refusals {
		if !unreadable[r.socket] {
			unreadable[r.socket] = true
			pl.diags = append(pl.diags, aggrega.Diagnostic{
				Severity: "warn",
				Code:     "socket-selector-unreadable",
				Message: fmt.Sprintf("the selector of the %s socket %s declares does not read, so no part is wired to it: %s",
					r.socket, p.Component, r.err),
			})
		}
	}
	if socket == "" {
		return keep(unplacedReason(pl.target, p.Component, p.Match != nil, passed), true)
	}
	pl.socket, pl.passed = socket, passed
	return pl
}

// singleHolder is the part holding the one connection a socket drawing a
// single connection draws: the placement of its first node and that node's
// reference, why it holds it, and, once wired, its key and its connection's
// alias.
type singleHolder struct {
	placement  int
	ref        string
	why        string
	key, alias string
}

// heldOut is a node whose part stays in the flow because another part holds
// the single connection of its socket: its placement, and the positions of its
// decision and of the diagnostic reporting it.
type heldOut struct {
	placement, decision, diag int
}

// holdSingleSockets chooses, for each socket of [Platform.Single] that parts
// were placed in, the part holding its single connection, as [SingleSocket]
// describes, and marks every node of every other part placed there as held out
// of it. Nodes the format names one part by are one part. It returns the
// holders by socket.
func holdSingleSockets(p *Platform, placements []placement, labels map[string]map[string]string) map[string]*singleHolder {
	holders := map[string]*singleHolder{}
	for _, s := range p.Single {
		if _, chosen := holders[s.Name]; chosen {
			continue
		}
		// The parts placed in the socket, each by the placement of its first
		// node, in the order of the nodes.
		var parts []int
		first := map[string]int{}
		partOf := map[int]int{}
		for i, pl := range placements {
			if pl.socket != s.Name {
				continue
			}
			if pl.reach.Name != "" {
				if at, seen := first[pl.reach.Name]; seen {
					partOf[i] = at
					continue
				}
				first[pl.reach.Name] = i
			}
			partOf[i] = i
			parts = append(parts, i)
		}
		if len(parts) == 0 {
			continue
		}
		holder := &singleHolder{placement: parts[0]}
		preferred := false
		for _, i := range parts {
			if carriesEvery(labels[placements[i].target], s.Prefer) {
				holder.placement, preferred = i, true
				break
			}
		}
		prefer := labelList(s.Prefer)
		switch {
		case preferred && prefer != "":
			holder.why = "its Component carries " + prefer
		case prefer != "":
			holder.why = "it is the first of the parts placed there in the flow, none of whose Components carries " + prefer
		default:
			holder.why = "it is the first of the parts placed there in the flow"
		}
		holder.ref = placements[holder.placement].ref
		if len(parts) > 1 {
			holders[s.Name] = holder
		}
		for i, at := range partOf {
			if at != holder.placement {
				placements[i].heldBy = s.Name
			}
		}
	}
	return holders
}

// carriesEvery reports whether labels carries every label of want.
func carriesEvery(labels, want map[string]string) bool {
	for k, v := range want {
		if have, ok := labels[k]; !ok || have != v {
			return false
		}
	}
	return true
}

// labelList writes labels as key: value pairs, sorted by key and joined with
// "and".
func labelList(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+": "+labels[k])
	}
	return strings.Join(pairs, " and ")
}

// heldOutReason says why a node's part stays in the flow when another part
// holds the single connection of the socket it was placed in, and, where the
// flow is carried and the format says so, how the flow reaches the node's part
// as the node is written.
func heldOutReason(p *Platform, carried bool, tree json.RawMessage, pl placement, holder *singleHolder) string {
	reason := fmt.Sprintf("it maps to %s, which satisfies the selector of the %s socket of %s, but that socket draws a "+
		"single connection, which reaches %s, wired from node %s since %s",
		pl.target, pl.heldBy, p.Component, holder.key, holder.ref, holder.why)
	if !carried || p.Standing == nil {
		return reason
	}
	through, how := p.Standing(tree, Part{Node: pl.node, Socket: pl.heldBy, Key: holder.key, Name: pl.reach.Name,
		Alias: holder.alias})
	switch {
	case how == "":
	case through:
		reason += fmt.Sprintf("; as its node is written, the flow reaches it through the connection %s: %s", holder.alias, how)
	default:
		reason += fmt.Sprintf("; as its node is written, the flow reaches it other than through the connection %s: %s",
			holder.alias, how)
	}
	return reason
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

// partModel reads, for a part wired to a socket of the platform, the model its
// node selects, where the draft writes one: asked is false unless the format
// reads models, the part's Component is referenced and the workspace's catalog
// says that Component declares a value named model. Where asked, model is the
// name to write there, or, where it is empty, why completes "the node …" with
// why none is written — the format's reason, or, for a name a YAML literal block
// cannot carry as written, the character it holds.
func partModel(p *Platform, view *aggrega.CatalogProjection, tree json.RawMessage, node Node,
	entity aggrega.EmittedEntity) (asked bool, model, why string) {
	if p.Model == nil || entity.Mode != "reference" {
		return false, "", ""
	}
	entry := catalogEntry(view, entity.Name)
	if entry == nil || !declaresValue(entry, modelValue) {
		return false, "", ""
	}
	name, why := p.Model(tree, node)
	switch {
	case name == "" && why == "":
		return true, "", noModelField
	case name == "":
		return true, "", why
	}
	if problems := blockProblems(name); len(problems) > 0 {
		return true, "", fmt.Sprintf("names a model whose name holds a %s, which a YAML literal block cannot carry as "+
			"written", problems[0].what)
	}
	return true, name, ""
}

// declaresValue reports whether a catalog Component's entry lists value among
// the key names its values declare.
func declaresValue(entry *aggrega.CatalogComponent, value string) bool {
	for _, v := range entry.Values {
		if v == value {
			return true
		}
	}
	return false
}

// modelNotNamed reports a referenced part whose values.model the draft leaves
// for the author, as partModel found it.
func modelNotNamed(ref string, node Node, key, component, why string) aggrega.Diagnostic {
	return aggrega.Diagnostic{
		Severity: "warn",
		Code:     "model-not-named",
		Message: fmt.Sprintf("values.%s of %s, the name of the model %s serves, is left for the author, since node %s "+
			"(%s) %s; a Release cannot write it", modelValue, key, component, ref, node.Type, why),
		SourceRef: ref,
	}
}

// egressAllowlistAttached reports that the attach_egress_allowlist setting
// attached core/egress-allowlist to the platform instance keyed key, drafted
// from the source named source, and what of it the Release sets.
func egressAllowlistAttached(key, source string) aggrega.Diagnostic {
	return aggrega.Diagnostic{
		Severity: "info",
		Code:     "feature-attached",
		Message: fmt.Sprintf("%s carries %s, attached as %s, which lets it connect only to the components it is wired "+
			"to; the import sets none of its infrastructure: the Release sets %s on that attachment, without which "+
			"the build halts, and %s for each connection whose target the cluster does not run", key, egressAllowlist,
			egressAllowlistKey, egressAffirmation, egressRanges),
		SourceRef: source,
	}
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
	// definition is the flow the definition value carries, its credentials
	// emptied, before draftedText writes it; empty where the flow is not carried.
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
	// left is what of each part the Release supplies, by the part's key, each in
	// the order its wiring named it.
	left map[string][]LeftItem
}

// connection is one connection a socket of the platform draws: its alias and the
// key of the part it reaches.
type connection struct{ alias, target string }

// platformPart is one part of the flow drafted as a Component of its own: where
// its Component takes the name of a model, model is the one its node selects,
// written into its values, or modelLeft why the draft leaves it for the author.
type platformPart struct {
	key       string
	decision  aggrega.ImportDecision
	entity    aggrega.EmittedEntity
	model     string
	modelLeft string
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

// leave records what of the part keyed key the Release supplies, each item once,
// in the order given.
func (inst *platformInstance) leave(key string, items ...LeftItem) {
	if inst.left == nil {
		inst.left = map[string][]LeftItem{}
	}
	for _, item := range items {
		if !slices.Contains(inst.left[key], item) {
			inst.left[key] = append(inst.left[key], item)
		}
	}
}

// wired reports whether any socket of the instance draws a connection.
func (inst *platformInstance) wired() bool {
	return len(inst.parts) > 0
}

// carriedWhere names, in each report of a credential value, the flow a platform
// carries.
const carriedWhere = "in the flow the draft carries"

// carry prepares the flow one source carries into the instance: the values the
// format writes anew written, its credential values written empty or carried as
// removal — the empty_credentials setting — decides, each that is wholly a
// variable reference ([IsVariableReference]) left as written, its carriage returns
// written as line feeds, and its identifier. It returns the diagnostics the
// preparation raised: one per credential value it emptied and one per value it
// carried holding something, each naming the node and the field; one naming how
// many carriage returns it wrote as line feeds; and, where the flow cannot be
// carried, why — a value to write anew, or one the setting writes empty, that
// the source's bytes do not hold where its tree does, a source not read as JSON
// whose bytes with those values written do not read back as its tree with
// exactly those values written, or a character a YAML literal block cannot carry
// as written. syntax is the syntax the source is parsed as and reparse parses
// bytes as it: a source of a syntax other than JSON whose carriage returns are
// written as line feeds is carried only where it reads back as the same tree.
// Where the source gives its flow no identifier a draft can carry, one is
// derived from the carried bytes — or, where a value could not be located and
// the flow is not carried, from its nodes' identifiers, so nothing a credential
// value could reach decides it.
func (inst *platformInstance) carry(p *Platform, format, syntax string, src aggrega.ImportSource, tree json.RawMessage,
	nodes []Node, refOf func(string) string, edits []Edit, removal credentialRemoval,
	reparse func([]byte) (json.RawMessage, error)) []aggrega.Diagnostic {
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
	definitionAt := "values." + p.DefinitionValue
	if p.Module != nil {
		definitionAt = "parameters." + p.Module.Parameter
	}
	writer := sourceWriter{empty: p.EmptyInSource, reparse: reparse}
	scrubbed, credentialDiags, err := carryCredentials(removal, src.Bytes, tree, edits, creds, carriedWhere, refOf, writer,
		true)
	if err != nil {
		why := "so what the draft writes for its parts could not be written"
		var unlocated credentialNotLocated
		if errors.As(err, &unlocated) {
			why = "so a credential it holds could not be kept out of the draft"
		}
		diags = append(diags, aggrega.Diagnostic{
			Severity:  "warn",
			Code:      "definition-not-carried",
			Message:   fmt.Sprintf("%s is not carried in %s: %s, %s", flow, definitionAt, err.Error(), why),
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
	diags = append(diags, credentialDiags...)

	text, returns := lineFeedsFor(string(scrubbed))
	var refused []aggrega.Diagnostic
	if returns > 0 {
		reads := "a JSON reader reads the carried flow as the same value"
		if syntax != "json" {
			reads = "the host's parser reads the carried flow back as the same tree"
		}
		if syntax == "json" || readAlike(reparse, []byte(text), scrubbed) {
			diags = append(diags, aggrega.Diagnostic{
				Severity: "warn",
				Code:     "carriage-returns-rewritten",
				Message: fmt.Sprintf("%s holds %d carriage return(s), each written as a line feed in %s, since a YAML "+
					"literal block cannot carry one: %s", flow, returns, definitionAt, reads),
				SourceRef: src.Name,
			})
		} else {
			refused = append(refused, aggrega.Diagnostic{
				Severity: "warn",
				Code:     "definition-not-carried",
				Message: fmt.Sprintf("%s is not carried in %s: it holds %d carriage return(s), which a YAML literal block "+
					"cannot carry, and written as line feeds it does not read back as the same tree", flow, definitionAt,
					returns),
				SourceRef: src.Name,
			})
		}
	}
	for _, pr := range blockProblems(text) {
		refused = append(refused, aggrega.Diagnostic{
			Severity: "warn",
			Code:     "definition-not-carried",
			Message: fmt.Sprintf("%s is not carried in %s: it holds a %s (%d in all, the first at line %d, "+
				"column %d), which a YAML literal block cannot carry as written",
				flow, definitionAt, pr.what, pr.count, pr.line, pr.column),
			SourceRef: src.Name,
		})
	}
	if len(refused) > 0 {
		// Nothing is carried, so no value is carried as written.
		kept := diags[:0]
		for _, d := range diags {
			if d.Code != "credential-carried" {
				kept = append(kept, d)
			}
		}
		diags = append(kept, refused...)
	} else {
		inst.definition = text
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

// platformDraft assembles the drafted Aggregate of a platform draft: each
// instance of the platform, followed by the parts it is wired to. titles are
// the names the sources it carries give themselves, one per instance.
type platformDraft struct {
	settings  importSettings
	format    string
	platform  *Platform
	instances []*platformInstance
	titles    []string
}

// entity renders the Aggregate as a draft EmittedEntity carrying hand-built
// YAML, so key order is fixed and the output is byte-identical across runs. It
// is named after the request in and the titles of the sources it carries, as
// [DraftName] names it.
func (d *platformDraft) entity(in aggrega.ImportInput) aggrega.EmittedEntity {
	p := d.platform
	if p.Module != nil {
		return d.moduleEntity(in)
	}
	name := draftName(d.settings, in, d.titles)
	var b strings.Builder
	b.WriteString("apiVersion: " + apiVersion + "\n")
	b.WriteString("kind: Aggregate\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: " + name + "\n")
	b.WriteString("  version: 0.1.0\n")
	b.WriteString("  description: " + draftDescription(d.format) + "\n")
	b.WriteString("spec:\n")
	b.WriteString("  components:\n")
	attach := d.settings.AttachEgressAllowlist
	for _, inst := range d.instances {
		// What no source supplies of the instance and its parts is left for the
		// Release on the Release's own entries (instanceLeft), not here.
		b.WriteString("    " + inst.key + ":\n")
		b.WriteString("      type: " + p.Component + "\n")
		if attach {
			b.WriteString("      features:\n")
			b.WriteString(CommentLine("        ", "Lets "+inst.key+" connect only to the components it is wired to, through a "+
				"network policy; compose has no network policy, so a compose build halts naming it."))
			b.WriteString("        " + egressAllowlistKey + ":\n")
			b.WriteString("          type: " + egressAllowlist + "\n")
		}
		if inst.wired() {
			// An attachment of the import's own target is declared; with no target,
			// each is shown commented out; one of another target is left out.
			inFeatures := attach
			for _, a := range p.TargetAttachments {
				switch {
				case in.Target == "":
					writeTargetAttachment(&b, inst.key, a, inFeatures, false)
				case a.Target == in.Target:
					if !inFeatures {
						b.WriteString("      features:\n")
						inFeatures = true
					}
					writeTargetAttachment(&b, inst.key, a, true, true)
				}
			}
		}
		b.WriteString("      values:\n")
		if inst.idNote != "" {
			b.WriteString(CommentLine("        ", p.IdentifierValue+" is "+inst.idNote+"."))
		}
		b.WriteString("        " + p.IdentifierValue + ": '" + inst.identifier + "'\n")
		if inst.carried {
			b.WriteString("        " + p.DefinitionValue + ": " + draftedText(inst.definition, 8))
		} else {
			b.WriteString(CommentLine("        ", p.DefinitionValue+" is left unset: the import's diagnostics say why the flow "+
				"is not carried."))
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
				b.WriteString(CommentLine("    ", part.decision.SwapHint))
			}
			b.WriteString("    " + part.key + ":\n")
			b.WriteString("      type: " + part.entity.Name + "\n")
			switch {
			case part.model != "":
				b.WriteString("      values:\n")
				b.WriteString("        " + modelValue + ": " + draftedText(part.model, 8))
			case part.modelLeft != "":
				b.WriteString(CommentLine("      ", "values."+modelValue+", the name of the model "+part.entity.Name+
					" serves, is left for the author, since the node this part stands for "+part.modelLeft+"."))
			}
		}
	}
	return aggrega.EmittedEntity{Kind: "Aggregate", Name: name, Mode: "draft", Body: b.String()}
}

// writeTargetAttachment writes, inside the platform's instance keyed key, the
// lines that declare a, beneath a comment saying which target needs them and
// why. Where live is set the import was given a's target, and the lines are
// written as declarations; otherwise each is commented out in place, so an author
// building for that target uncomments them where they stand. Where inFeatures is
// set the instance already declares features, so the lines are written inside
// that block, without a second features key; a live attachment is always
// written so, the caller declaring features first.
func writeTargetAttachment(b *strings.Builder, key string, a TargetAttachment, inFeatures, live bool) {
	indent, lines := "      ", []string{
		"features:",
		"  " + a.Slot + ":",
		"    type: " + a.Type,
		"    values:",
		"      key: " + a.Key,
	}
	if inFeatures {
		indent, lines = "        ", []string{
			a.Slot + ":",
			"  type: " + a.Type,
			"  values:",
			"    key: " + a.Key,
		}
	}
	if live {
		b.WriteString(CommentLine(indent, "For the "+a.Target+" target, the lines below declare "+a.Slot+" on "+key+": "+
			a.Why+"."))
		for _, line := range lines {
			b.WriteString(indent + line + "\n")
		}
		return
	}
	b.WriteString(CommentLine(indent, "For the "+a.Target+" target only, uncomment the lines below, which declare "+
		a.Slot+" on "+key+": "+a.Why+"."))
	for _, line := range lines {
		b.WriteString(CommentLine(indent, line))
	}
}
