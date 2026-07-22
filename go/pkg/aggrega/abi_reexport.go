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

// Policy socket. The graph is the full capability-agnostic IR graph; the
// IR types are re-exported alongside the policy envelope so plugin authors build
// and read the same IR shape the compiler emits.
type (
	PolicyInput       = abi.PolicyInput
	PolicyOutput      = abi.PolicyOutput
	PolicyResult      = abi.PolicyResult
	PrecalculatedPath = abi.PrecalculatedPath
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
	RenderNodeInput            = abi.RenderNodeInput
	RenderNodeOutput           = abi.RenderNodeOutput
	IRNodeProjection           = abi.IRNodeProjection
	NodeProvenance             = abi.NodeProvenance
	CatalogProjection          = abi.CatalogProjection
	SourceProjection           = abi.SourceProjection
	WorkloadProjection         = abi.WorkloadProjection
	SourceFile                 = abi.SourceFile
	SourceListing              = abi.SourceListing
	SourceEntry                = abi.SourceEntry
	SourcePackRequest          = abi.SourcePackRequest
	SourcePackResponse         = abi.SourcePackResponse
	SourceFileRequest          = abi.SourceFileRequest
	SourceFileResponse         = abi.SourceFileResponse
	FetchExternalInputResponse = abi.FetchExternalInputResponse
	RenderedFile               = abi.RenderedFile
	AssembleBoundaryInput      = abi.AssembleBoundaryInput
	AssembleBoundaryOutput     = abi.AssembleBoundaryOutput
	RenderedNode               = abi.RenderedNode
	RenderEdge                 = abi.RenderEdge
	SubChart                   = abi.SubChart
)

// CLI socket. A cli plugin extends the compiler's command surface: the host
// resolves a subcommand to a guest and hands it the invocation's argument
// vector on the cli_run export. CLIRunInput carries that argument vector; the
// guest streams stdout/stderr through the streaming host functions rather than
// a wire payload, so the socket declares an input shape only.
type (
	CLIRunInput = abi.CLIRunInput
)

// Describe socket. Every plugin returns a descriptor from its describe()
// export, which the host reads once at load: the ABI-compatibility handshake,
// the plugin type, the offered exports, and the optional per-type surfaces (a
// provider's render-behavior contract, a CLI plugin's commands and their
// flags, a settings schema, authorship and build-provenance metadata). A guest
// constructs a typed PluginDescriptor and marshals it to the wire bytes through
// [MarshalDescriptor] rather than hand-assembling the JSON.
type (
	PluginDescriptor       = abi.PluginDescriptor
	PluginExport           = abi.PluginExport
	PluginCLICommand       = abi.PluginCLICommand
	PluginCommandFlag      = abi.PluginCommandFlag
	PluginDescriptorMeta   = abi.PluginDescriptorMeta
	ProviderContract       = abi.ProviderContract
	InputKinds             = abi.InputKinds
	ArtifactDelimiters     = abi.ArtifactDelimiters
	AssemblyNameAnnotation = abi.AssemblyNameAnnotation
)

// Parser socket. A parser guest turns a foreign source text into a canonical
// structured tree on the parse export (ParseInput to ParseOutput): it receives
// the format name, the base64 source bytes, and an optional opaque options
// document, and returns the tree plus optional diagnostics and source spans. A
// parser advertises the formats it handles as parse:<format> capabilities in
// its descriptor.
type (
	ParseInput          = abi.ParseInput
	ParseOutput         = abi.ParseOutput
	ParseDiagnostic     = abi.ParseDiagnostic
	NodeSpan            = abi.NodeSpan
	SourceSpan          = abi.SourceSpan
	ParseSourceResponse = abi.ParseSourceResponse
)

// Importer socket. An importer guest turns a foreign infrastructure
// description into Aggrega entities on the import export (ImportInput to
// ImportOutput): it receives the source format, the base64 primary bytes, any
// named sources, and an optional opaque settings document, and returns the
// emitted entities, the mapping decisions behind them, any diagnostics, and any
// scaffold files. An importer advertises the formats it handles as
// import:<format> capabilities in its descriptor.
type (
	ImportInput        = abi.ImportInput
	ImportOutput       = abi.ImportOutput
	ImportSource       = abi.ImportSource
	EmittedEntity      = abi.EmittedEntity
	ImportDecision     = abi.ImportDecision
	DecisionSourceNode = abi.DecisionSourceNode
	ImportCandidate    = abi.ImportCandidate
	CandidateSignals   = abi.CandidateSignals
	DecisionChoice     = abi.DecisionChoice
	Diagnostic         = abi.Diagnostic
	ScaffoldFile       = abi.ScaffoldFile
)

// Cross-boundary edge linking. A guest that wires uses edges whose ends
// assemble into different dispatch groups implements the link_publish and
// link_resolve exports, reading the host-assembled LinkEdge and returning the
// producer-side and consumer-side artifact wiring as LinkPatch lists. The patch
// ops carry the shared pipeline patch-operation shape opaquely, so a guest
// authors each op's raw JSON directly rather than through a typed op value.
type (
	LinkEndpoint      = abi.LinkEndpoint
	LinkEdge          = abi.LinkEdge
	LinkPatch         = abi.LinkPatch
	LinkPublishInput  = abi.LinkPublishInput
	LinkPublishOutput = abi.LinkPublishOutput
	LinkResolveInput  = abi.LinkResolveInput
	LinkResolveOutput = abi.LinkResolveOutput
)
