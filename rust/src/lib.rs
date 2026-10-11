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
    write_stderr, write_stdout, HostError,
};

// Every socket type is generated from the canonical schemas into the portable
// aggrega-abi crate and re-exported here, so plugin authors keep the flat
// `aggrega_pdk_rust::<Type>` spelling while the wire shape stays single-sourced
// in aggrega-abi. The policy socket's graph is the full IR graph, so
// the IR types are re-exported alongside the policy envelope.
pub use aggrega_abi::{
    ArtifactDelimiters, AssembleBoundaryInput, AssembleBoundaryOutput, AssemblyNameAnnotation,
    AttachmentConnection, CLIRunInput, CatalogMetadata,
    CompilationContext, FieldProvenance,
    IRBoundaryFeature, IREdge, IRFeatureEntry, IRGraph, IRMetadata, IRNode, IRNodeProjection,
    IRProvenance, IRSpec, InputKinds, LinkEdge, LinkEndpoint, LinkPatch, LinkPublishInput,
    LinkPublishOutput,
    LinkResolveInput, LinkResolveOutput, LinterInput, LinterOutput, Location, Maintainer,
    MutatorGraph,
    MutatorInput, MutatorNode, MutatorOutput, NameBoundInput, NameBoundOutput, NodeProvenance,
    NodeSpan, DerivedNameVerdict, OmitEntry,
    ParseInput, ParseOutput,
    PluginCLICommand, PluginCommandFlag, PluginDescriptor, PluginDescriptorMeta, PluginExport,
    Policies,
    PolicyInput, PolicyOutput, PrecalculatedPath, ProviderContract, RenderEdge, RenderFact,
    RenderNodeInput, RenderNodeOutput, RenderedFile, RenderedNode,
    SourceEntry, SourceFile, SourceFileRequest, SourceIgnore,
    SourceFileResponse, SourceListing, SourcePackRequest, SourcePackResponse, SourceProjection,
    SourceSpan, Span, SubChart, WorkloadProjection,
    CandidateSignals, DecisionChoice, DecisionSourceNode, Diagnostic, EmittedEntity,
    ImportCandidate, ImportDecision, ImportInput, ImportOutput, ImportSource, ScaffoldFile,
    FetchExternalInputResponse, ParseSourceResponse,
};

// Diagnostic severities, re-exported from aggrega-abi so a plugin author writes
// aggrega_pdk_rust::SEVERITY_* and reports from the same closed, single-sourced
// vocabulary the host routes on.
pub use aggrega_abi::{SEVERITY_HALT, SEVERITY_INFO, SEVERITY_WARN};

// Host-function error codes, re-exported from aggrega-abi so a plugin author
// writes aggrega_pdk_rust::CODE_* and matches a HostError's code against the
// same single-sourced vocabulary the host emits.
pub use aggrega_abi::{
    CODE_FETCH_FAILED, CODE_FETCH_INPUT_UNAVAILABLE, CODE_FETCH_REQUEST_UNREADABLE,
    CODE_NO_PARSER_FOR_FORMAT, CODE_NO_SOURCE_PARSER, CODE_PARSE_FAILED,
    CODE_PRODUCER_CONSUMPTION_REENTRY, CODE_REQUEST_UNDECODABLE, CODE_REQUEST_UNREADABLE,
    CODE_SOURCE_ESCAPE, CODE_SOURCE_READ_FAILED, CODE_SOURCE_ROOT_UNAVAILABLE,
    CODE_UNKNOWN_SELECTION_OP,
};
pub use version::API_VERSION;
