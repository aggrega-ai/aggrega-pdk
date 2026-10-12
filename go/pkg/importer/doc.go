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
//     parse a source, and encodes what [Import] returns.
//
// The package gives them the rest. [Import] parses each source through the
// callback, walks it, scores every node against the candidate standard-library
// components the package carries, decides whether to reference a component or
// draft a new one, renders each drafted Component and the Aggregate composing
// them, and renders the whole result into files. [NodeList], [EdgeList] and
// [ObjectEntries] read the shapes a walker commonly meets in a canonical tree.
//
// # Scoring and decisions
//
// The scorer is deterministic and holds no model. It weighs a node against each
// candidate through five signals combined as a fixed weighted sum: the mapping
// arm naming the candidate (0.50), the node's category matching the candidate's
// (0.20), the node's implied traits agreeing with the candidate's domain labels
// (0.15), the overlap of the node's field names with the candidate's (0.10), and
// the similarity of the node's vendor token to the candidate's name (0.05). A
// score of at least 0.85 is a HIGH match, at least 0.50 MEDIUM, at least 0.20
// LOW, and anything less NONE. The candidates scoring above zero are ranked by
// score and then by name, and at most four are reported per node.
//
// Every node is drafted as a new Component unless the reuse mode is
// auto-apply-above-threshold and its best candidate is a HIGH match scoring at
// or above the reuse threshold, in which case the candidate is referenced. A
// drafted node records how strongly its best candidate matched, and a HIGH or
// MEDIUM match is written into the drafts as an inline hint.
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
