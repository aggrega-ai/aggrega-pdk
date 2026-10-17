// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

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
//     a flow's own identifier and its credential values, and the matcher that
//     reads a socket's selector.
//
// The package gives them the rest. [Import] parses each source through the
// callback, walks it, scores every node against the Components of the
// workspace's catalog the request carries, decides whether to reference a
// component or draft a new one, renders each drafted Component and the Aggregate
// composing them, and renders the whole result into files. [NodeList], [EdgeList]
// and [ObjectEntries] read the shapes a walker commonly meets in a canonical tree.
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
// Every node is drafted as a new Component unless the reuse mode is
// auto-apply-above-threshold and its best candidate is a HIGH match scoring at
// or above the reuse threshold, in which case the candidate is referenced. A
// drafted node records how strongly its best candidate matched, and a HIGH or
// MEDIUM match is written into the drafts as an inline hint.
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
// MEDIUM match — satisfy. Every other node stays inside the flow and becomes no
// Component; its decision records the candidates it was weighed against, chooses
// nothing, and says that the flow keeps it. A part no socket admits stays in the
// flow too, and the import reports it. Where the
// workspace's catalog holds no Component of the platform's name, the platform is
// drafted all the same, no part is wired, and the import warns.
//
// The source is carried as its own bytes with every value in a field the format
// marks as a credential written empty, each one reported by its node and field
// and never by its value, and nothing else changed. Every "{{" in it is written
// {{"{{"}}, which an Aggregate's template reading turns back into the two
// braces, and it is written as a YAML literal block whose chomping keeps its
// trailing line breaks. A source holding a character such a block cannot carry
// as written — a carriage return among them — is not carried, and the import
// says which character and where. The platform's own inputs no source supplies
// are named in a comment above its instance, left for the Release.
//
// # Settings
//
// The importer reads the settings document an import attachment hands it. The
// host passes the document through without reading it. An absent document, or
// one that is not valid JSON, leaves every setting at its default.
//
//   - target_namespace: the namespace drafted entities are named under, as
//     <namespace>/<category>/<name>; default imported.
//   - reuse_mode: hints-only (the default), which references nothing, or
//     auto-apply-above-threshold.
//   - reuse_threshold: the score at or above which auto-apply references a
//     HIGH match; default 0.85.
//   - emit_hints: whether hints are written into the drafts as comments;
//     default true. The decision record carries them either way.
//   - render_mode: human (the default), one file per drafted Component plus
//     the Aggregate, or llm, the whole decision record as one JSON document.
//   - mapping_overrides: format, then node type, then a mapping with the keys
//     component, category, traits and vendor, taking precedence over the
//     importer's own mapping arm.
//   - layout: platform (the default), the platform carrying each source, or
//     nodes, every node drafted as a Component of its own. A format describing
//     no platform is drafted node by node whatever it says, and a value that is
//     neither is drafted as platform with a warning.
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
