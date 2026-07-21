// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Aggrega Plugin Development Kit for Rust.
//!
//! Plugin authors targeting the Aggrega Wasm plugin ABI consume this
//! crate to obtain typed socket payloads, the [`APIVersion`] contract
//! floor constant, and bindings to the Aggrega host functions
//! (`aggrega_log`, `aggrega_emit_warning`,
//! `aggrega_fetch_external_input`, and the CLI streaming emitters
//! `aggrega_cli_write_stdout` / `aggrega_cli_write_stderr`).
//!
//! Build a plugin to a `.wasm` artifact via:
//!
//! ```text
//! cargo build --release --target wasm32-unknown-unknown
//! ```
//!
//! See the crate-level README and the per-fixture examples under
//! `examples/` for working plugin sources.

pub mod describe;
pub mod host_funcs;
pub mod types;
pub mod version;

pub use describe::encode_descriptor;
pub use host_funcs::{
    cli_args, emit_warning, fetch_external_input, log, pack_source, parse_source, read_source_file,
    write_stderr, write_stdout,
};

// Every socket type is generated from the canonical schemas into the portable
// aggrega-abi crate and re-exported here, so plugin authors keep the flat
// `aggrega_pdk_rust::<Type>` spelling while the wire shape stays single-sourced
// in aggrega-abi. The policy socket's graph is the full IR graph, so
// the IR types are re-exported alongside the policy envelope.
pub use aggrega_abi::{
    ArtifactDelimiters, AssembleBoundaryInput, AssembleBoundaryOutput, AssemblyNameAnnotation,
    CLIRunInput, CatalogMetadata, CatalogProjection,
    CompilationContext, FieldProvenance,
    IRBoundaryFeature, IREdge, IRFeatureEntry, IRGraph, IRMetadata, IRNode, IRNodeProjection,
    IRProvenance, IRSpec, InputKinds, LinkEdge, LinkEndpoint, LinkPatch, LinkPublishInput,
    LinkPublishOutput,
    LinkResolveInput, LinkResolveOutput, LinterFinding, LinterInput, LinterOutput, Maintainer,
    MutatorGraph,
    MutatorInput, MutatorNode, MutatorOutput, NarrowingExemption, NodeProvenance, NodeSpan,
    ParseDiagnostic, ParseInput, ParseOutput,
    PluginCLICommand, PluginCommandFlag, PluginDescriptor, PluginDescriptorMeta, PluginExport,
    Policies,
    PolicyResult,
    PolicyInput, PolicyOutput, PrecalculatedPath, ProviderContract, RenderEdge, RenderNodeInput,
    RenderNodeOutput, RenderedFile, RenderedNode,
    SourceEntry, SourceFile, SourceFileRequest,
    SourceFileResponse, SourceListing, SourcePackRequest, SourcePackResponse, SourceProjection,
    SourceSpan, SubChart, WorkloadProjection,
    CandidateSignals, DecisionChoice, DecisionSourceNode, Diagnostic, EmittedEntity,
    ImportCandidate, ImportDecision, ImportInput, ImportOutput, ImportSource, ScaffoldFile,
};
pub use version::API_VERSION;
