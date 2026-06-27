// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package aggrega

import abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"

// The mutator and linter socket wire types are generated from the canonical
// schemas into the portable aggrega-abi module and re-exported here as aliases,
// so plugin authors writing against this PDK keep the aggrega.<Type> spelling
// while the wire shape stays single-sourced in aggrega-abi. The aliases are
// identical types (Go type aliases), so a value built here is the same value the
// abi codec marshals.

// Mutator socket.
type (
	MutatorInput       = abi.MutatorInput
	MutatorOutput      = abi.MutatorOutput
	MutatorGraph       = abi.MutatorGraph
	MutatorNode        = abi.MutatorNode
	CompilationContext = abi.CompilationContext
)

// Linter socket.
type (
	LinterInput        = abi.LinterInput
	LinterOutput       = abi.LinterOutput
	LinterFinding      = abi.LinterFinding
	Policies           = abi.Policies
	NarrowingExemption = abi.NarrowingExemption
)
