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

// Policy-validator socket. The graph is the full target-agnostic IR graph; the
// IR types are re-exported alongside the policy envelope so plugin authors build
// and read the same IR shape the compiler emits.
type (
	PolicyValidatorInput  = abi.PolicyValidatorInput
	PolicyValidatorOutput = abi.PolicyValidatorOutput
	PolicyResult          = abi.PolicyResult
	PrecalculatedPath     = abi.PrecalculatedPath
)

// IR graph (the policy socket's graph shape).
type (
	IRGraph           = abi.IRGraph
	IRMetadata        = abi.IRMetadata
	IRSpec            = abi.IRSpec
	IRNode            = abi.IRNode
	IREdge            = abi.IREdge
	IRFeatureEntry    = abi.IRFeatureEntry
	IRBoundaryFeature = abi.IRBoundaryFeature
	IRProvenance      = abi.IRProvenance
	FieldProvenance   = abi.FieldProvenance
	CatalogMetadata   = abi.CatalogMetadata
	Maintainer        = abi.Maintainer
)

// Provider render socket. A guest renders one IR node into output files on
// render_node (RenderNodeInput to RenderNodeOutput) and optionally merges the
// per-node outputs at one dispatch group (AssembleBoundaryInput to
// AssembleBoundaryOutput). A sourced provider reconstructs its source in memory
// from the input's SourceProjection. The guest returns each file's serialized
// bytes (RenderedFile.Content); the host writes them verbatim and runs no
// serializer.
type (
	RenderNodeInput        = abi.RenderNodeInput
	RenderNodeOutput       = abi.RenderNodeOutput
	IRNodeProjection       = abi.IRNodeProjection
	NodeProvenance         = abi.NodeProvenance
	CatalogProjection      = abi.CatalogProjection
	SourceProjection       = abi.SourceProjection
	WorkloadProjection     = abi.WorkloadProjection
	SourceFile             = abi.SourceFile
	RenderedFile           = abi.RenderedFile
	AssembleBoundaryInput  = abi.AssembleBoundaryInput
	AssembleBoundaryOutput = abi.AssembleBoundaryOutput
	RenderedNode           = abi.RenderedNode
	RenderEdge             = abi.RenderEdge
	SubChart               = abi.SubChart
)
