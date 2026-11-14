// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Package importer is the library an importer plugin is built on: everything
// about turning a foreign flow definition into draft Aggrega entities that does
// not depend on the format the definition is written in.
//
// An importer plugin serves one source format. Its author writes the part that
// knows the format:
//
//   - the describe() descriptor, advertising the import:<format> capability and
//     declaring the aggrega_parse_source host function on the import export;
//   - a walker, which lifts the [Node] and [Edge] values out of the canonical
//     tree the host's parser returns for one source;
//   - a mapping arm, the [Mapping] each node type of the format maps to;
//   - the syntactic format the host parses a source of the format as;
//   - the import export, which decodes the request, hands it to [Import] with
//     the [Format] holding the three above and a callback asking the host to
//     parse a source, and encodes what [Import] returns;
//   - for a format whose flows a platform runs as they are written, the
//     [Platform] describing it: the catalog Component that runs a flow, the
//     values it carries the flow and its identifier in, where the format keeps
//     a flow's own identifier, its credential values and the model a node
//     selects, the matcher that reads a socket's selector, and how the carried
//     flow reaches each part wired to the platform.
//
// The package gives them the rest. [Import] parses each source through the
// callback, walks it, scores every node against the Components of the
// workspace's catalog the request carries, decides whether to reference a
// component or draft a new one, renders each drafted Component, the Aggregate
// composing them and the Release deploying it, and renders the whole result into
// files. The Aggregate is named after the source, through the name the format's
// Title reads off it or the name of the directory or the file the import was
// handed, as [DraftName] names it; an importer drafting an Aggregate of its own
// names it through DraftName too. Every drafted Release is written by
// [DraftRelease], which names it after the drafted Aggregate it deploys: the
// package's own drafts and every importer drafting its own Aggregate draft the
// Release beside it through it, so a drafted Release reads alike whichever
// importer drafts it. It writes every value the Release must supply and no source
// holds as its key holding an empty string, with a comment naming it directly
// above, and every secret the draft attaches as a reference for the import's
// target ([Secret]), never as a value. [NodeList], [EdgeList] and [ObjectEntries] read the shapes
// a walker commonly meets in a canonical tree.
//
// # Scoring and decisions
//
// The candidates are the Components of the view of the workspace's own catalog
// the request carries in its Catalog field — the catalog the importer was
// selected from, enterprise catalogs included — and the package carries no
// candidate of its own. A request whose view is absent, or holds no Component,
// has no candidate: every node is drafted and none is referenced.
//
// The scorer is deterministic and holds no model. It weighs a node against each
// candidate through five signals combined as a fixed weighted sum: the mapping
// arm naming the candidate (0.50), the node's category matching the category
// the candidate's component.aggrega.ai/category label carries (0.20), the node's
// implied traits agreeing with the candidate's labels (0.15), the overlap of the
// node's field and credential names with the key names the candidate's values
// declare (0.10), and the similarity of the node's vendor token to the
// candidate's name (0.05). A candidate's provides keys and sockets are not
// weighed. A score of at least 0.85 is a HIGH match, at least 0.50 MEDIUM, at
// least 0.20 LOW, and anything less NONE. The candidates scoring above zero are
// ranked by score and then by name, and at most four are reported per node. The
// host's view holds one entry per Component name, so the order it lists them in
// decides nothing a decision records.
//
// A node whose best candidate is a HIGH match scoring at or above the reuse
// threshold references that candidate, under the default reuse mode,
// auto-apply-above-threshold; every other node is drafted as a new Component, and
// so is every node under the reuse mode hints-only. A drafted node records how
// strongly its best candidate matched, and a HIGH or MEDIUM match is written into
// the drafts as an inline hint.
//
// # The platform layout
//
// A format describing a [Platform] is drafted, by default, as the platform
// carrying each source: one instance of the platform's Component per source,
// its definition value holding the source as written and its identifier value
// the flow's own identifier, or one derived from the carried bytes where the
// source gives none. A node a mapping row maps, or whose best candidate is at
// least a MEDIUM match, is a part of the flow: it becomes a Component of its
// own, referenced or drafted as above, wired to the first socket of the
// platform's catalog entry whose selector the labels of that Component — or of
// its best candidate, for a drafted one, where that candidate is at least a
// MEDIUM match — satisfy. A format may say that a node names a part rather than
// being one, as an MCP Tools node names the server it calls: such a node is a
// part whatever its scores, nodes naming one part reach one Component wired
// once, and its connection is written under the name the flow gives the part
// wherever a connection's alias can take that name. A format may also refuse a
// node a socket, as a platform reaching a tools connection only through a node
// naming it refuses that socket to every other node: the part passes that socket
// over for the next one whose selector its Component satisfies. And a format may
// say that a socket draws a single connection, as one does whose connection the
// platform delivers under fixed variable names: of the parts placed in it, the
// first whose Component carries the labels the format prefers holds the
// connection — the first part, where none does — and every other stays in the
// flow, reported with the part holding the connection and with how the flow
// reaches it as its node is written. Every other node stays inside the flow and
// becomes no Component; its decision records the
// candidates it was weighed against, chooses nothing, and says that the flow
// keeps it. A part no socket admits, or that every socket admitting it refuses,
// stays in the flow too, and the import reports it with why. Where the
// workspace's catalog holds no Component of the platform's name, the platform
// is drafted all the same, no part is wired, and the import warns.
//
// The source is carried as its own bytes. The format locates every value in it
// that can carry a credential, each of one class — what the format marks as a
// credential, a member of an MCP server's configuration, or an input the format
// marks as none that can carry one — and the empty_credentials setting decides,
// in one place for every format, which classes are written empty: by default
// none, so each value is carried as written and named in a credential-carried
// warning, by its node and field and never by its value, which also names the
// setting value that writes it empty; each value the setting writes empty is
// reported under credential-emptied. A value that is wholly a variable reference
// ([IsVariableReference]) — the bare name ${NAME}, ${env:NAME} or ${input:id} —
// holds no value, and is left as written under every setting and named in no
// warning; a reference carrying text after an operator, such as ${NAME-value},
// ${NAME:-value} or ${NAME?message}, holds that text and is located like any
// other value. [CarryCredentials] is the same decision for
// an importer that drafts values of its own, which hands it only the values its
// format's own reading of a reference says hold one. Nothing else is changed but what
// the format writes so that the flow reaches each wired part through its
// connection — the name of the variable the platform delivers for the
// connection, written into the field that reads it, an address the platform
// writes for itself, written empty, or what the node would send in place of
// what the connection supplies, written empty — and what it writes for a node
// the draft wires to no socket that a connection reaches as the node is
// written, which is wiring and not removal, whatever the setting says — and
// each value the platform's own tools read only in a spelling other than the
// source's, written in that spelling, the same value, which the import reports
// in a definition-respelled warning — but never a value the wiring writes, nor
// one that can carry a credential, which the setting alone decides. The import reports how the flow reaches
// each wired part, and a part the flow reaches other than through its connection. Each carriage return is written
// as a line feed, which a JSON reader reads alike, and the import says how many.
// Every "{{" is written {{"{{"}}, which an Aggregate's template reading turns
// back into the two braces, and the source is written as a YAML literal block
// whose chomping keeps its trailing line breaks. A source holding any other
// character such a block cannot carry as written is not carried, and the import
// says which character and where. The platform's own inputs no source supplies,
// and what of each wired part the Release supplies, are left for the Release on
// the drafted Release's entries — the instance's and each part's — each under the
// key path it is written at. The drafted Release runs the platform from the image
// its format names, written explicitly; where the import's target is compose, it
// sets infra.published_port for each port the platform publishes on the host's
// loopback address, beneath a comment saying so, never at a number another port
// of the Release takes; and it
// attaches the platform's own secrets
// and the credential it presents on each connection of a socket its format names
// a credential variable for, as references; a connection of any other socket
// leaves its credential for the author, in a comment.
//
// Those values are located in a source's bytes read as JSON, unless its format
// writes them in its own syntax: a format whose sources are not JSON documents writes
// each value the empty_credentials setting writes empty in the source's bytes,
// and the import carries the result only where the host's parser reads it back
// as the source's tree with exactly those values emptied — and otherwise
// carries nothing of the source, saying why. A source the setting leaves as
// written is carried byte for byte, its format never asked.
//
// A platform running as a whole installation the catalog assembles as one
// Aggregate is drafted in the module form: the drafted Aggregate composes one
// entry of that Aggregate per source, under aggregates, carrying the source in
// the Aggregate's parameter that takes it. The module wires every connection of
// the platform itself, so no part of the flow is wired and no Component is
// drafted; every node stays in the flow, and what the format says the platform
// configures for itself — a model and its provider, a tool, a knowledge base, a
// plugin — is reported in the node's decision and in a
// part-configured-in-platform warning. No identifier is written; the platform's
// inputs no source supplies are left on the drafted Release's routing entry for
// the module, and a comment above the Aggregate's entry says that an import sees
// the Components of the workspace's catalog and none of its Aggregates, so it
// does not check that the catalog holds the module.
//
// A part the draft references whose Component declares a value named model —
// as every model Component of the standard library does, taking the name of the
// model it serves there — is written with the name of the model its node
// selects, as the format reads it off the node, through the writer the
// definition goes through. Where the node names none the draft can write, a
// comment leaves values.model for the author and the import reports it: a
// Release cannot write a value. A secret the draft attaches is drafted for the
// target the import is given, which the request carries, in the one form
// [Secret.Form] gives it: on a target reading every secret the Release alone
// attaches under one default key, the Aggregate declares it with a key of its own,
// inside the instance or on its connection, so no two secrets arrive under one
// key; an import for another target leaves those lines out, and an import naming
// no target shows them in a comment where they belong, commented out, for an
// author building for that target to uncomment ([Secret.Declaration]).
//
// Where the attach_egress_allowlist setting is true, each instance carries
// core/egress-allowlist under features, the decision of each wired part says
// that attachment admits its connection, and the import reports the attachment
// and what of it the Release sets. The draft never sets the Feature's
// infrastructure, the operator's affirmation that the cluster enforces network
// policy included.
//
// # Settings
//
// The importer reads the settings document an import attachment hands it. The
// host passes the document through without reading it. An absent document, or
// one that is not valid JSON, leaves every setting at its default.
//
//   - target_namespace: the namespace drafted entities are named under, as
//     <namespace>/<category>/<name> for a Component and <namespace>/<name> for
//     the Aggregate, its <name> named after the source as [DraftName] names it;
//     default imported.
//   - reuse_mode: auto-apply-above-threshold (the default), which references a
//     HIGH match scoring at or above reuse_threshold, or hints-only, which
//     references nothing and records each match as a hint.
//   - reuse_threshold: the score at or above which auto-apply references a
//     HIGH match; default 0.85.
//   - emit_hints: whether hints are written into the drafts as comments;
//     default true. The decision record carries them either way.
//   - empty_credentials: none (the default), which writes no credential value
//     empty, marked, which writes empty what a format marks as a credential and
//     every member of an MCP server's configuration, or all, which writes empty
//     the inputs a format marks as no credential that can carry one as well. A
//     value carried is named in a warning. Any other value halts the import,
//     which drafts nothing; a draft made node by node reads no value, and so
//     does not read this setting.
//   - render_mode: human (the default), one file per drafted Component plus
//     the Aggregate and the Release, or llm, the whole decision record as one
//     JSON document.
//   - mapping_overrides: format, then node type, then a mapping with the keys
//     component, category, traits and vendor, taking precedence over the
//     importer's own mapping arm.
//   - layout: platform (the default), the platform carrying each source, or
//     nodes, every node drafted as a Component of its own. A format describing
//     no platform is drafted node by node whatever it says, and a value that is
//     neither is drafted as platform with a warning.
//   - attach_egress_allowlist: whether each platform instance the platform
//     layout drafts carries core/egress-allowlist, attached as egress-allowlist,
//     so the platform connects only to the components it is wired to; default
//     false. The draft sets none of the Feature's infrastructure: the comment
//     above the drafted Release's entry for the instance names the two members
//     the Release sets, infra.control_plane_present, a boolean, and
//     infra.admitted_destinations, a list, and the entry writes neither. A draft made
//     node by node attaches nothing, and the import warns where this is true.
//
// # Building and testing
//
// The package builds for GOOS=wasip1 and on the host toolchain alike, and it
// reaches no host function itself: the only way it obtains a source tree is the
// parse callback it is handed. An importer's logic is therefore tested on the
// host by handing [Import] a parse function of the test's own.
//
// Given the same request, the same [Format] and the same trees from the parse
// callback, [Import] returns byte-identical output.
package importer
