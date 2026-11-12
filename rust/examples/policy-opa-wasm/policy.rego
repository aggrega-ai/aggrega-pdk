# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Cognitio s.r.l.
#
# Reference policy for the policy-opa-wasm adapter. It is compiled to a
# WebAssembly module with OPA's own toolchain and committed as policy.wasm,
# which the adapter embeds and drives inside the guest. Regenerate policy.wasm
# after editing this file with the command documented in the README.
#
# The whole policy payload is the OPA `input` document, so the IR graph is
# reached at input.graph. The attachment's settings object is the OPA `data`
# document, so an operator-supplied tunable is read at data.<key>.
package aggrega.policy

import rego.v1

# deny halts the build. This reference policy denies when the build graph
# carries more nodes than the operator permits, reading the ceiling from the
# attachment settings (data.max_nodes) and the node count from the IR graph
# (input.graph.spec.nodes). When max_nodes is unset the comparison is
# undefined and the rule contributes nothing, so the build proceeds.
deny contains msg if {
	count(input.graph.spec.nodes) > data.max_nodes
	msg := "the build graph exceeds the permitted node count"
}

# warn flags the build but lets it proceed. This reference rule fires whenever
# the graph declares routing edges, demonstrating a non-blocking verdict.
warn contains msg if {
	count(input.graph.spec.edges) > 0
	msg := "the build graph declares routing edges"
}
