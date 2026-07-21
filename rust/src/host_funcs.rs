// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Bindings to the Aggrega host functions.
//!
//! The `#[host_fn]` macro from `extism-pdk` declares a Wasm import
//! interface: from the guest's perspective the functions are imported
//! from the host; the macro emits the i64-pointer marshalling so
//! plugin authors write Rust-typed signatures.
//!
//! Safe wrappers ([`log`], [`emit_warning`], [`fetch_external_input`])
//! hide the `unsafe { ... }` block from plugin authors. Plugin code
//! calls `log(level, "message")` directly; the wrapper handles the
//! host-call invocation.

use aggrega_abi::{
    CLIRunInput, ParseInput, ParseSourceResponse, SourceFileRequest, SourceFileResponse,
    SourcePackRequest, SourcePackResponse, SourceProjection,
};
use base64::Engine as _;
use extism_pdk::{host_fn, warn, Error, Memory};
use serde_json::value::RawValue;

#[host_fn("extism:host/user")]
extern "ExtismHost" {
    fn aggrega_log(level: u64, message: String);
    fn aggrega_emit_warning(code: String, message: String, path: String);
    fn aggrega_fetch_external_input(url: String) -> String;
    fn aggrega_pack_source(request: String) -> String;
    fn aggrega_read_source_file(request: String) -> String;
    fn aggrega_parse_source(request: String) -> String;
}

// The streaming CLI emitters take an explicit (pointer, length) pair, so they
// are imported as raw Wasm functions rather than through the `#[host_fn]`
// macro. That macro marshals each argument to a single memory handle, which
// cannot express a two-integer (ptr, len) signature; the host reads exactly
// `len` bytes at `ptr`, so the guest passes the allocation's offset and length
// as two separate i64 arguments.
#[link(wasm_import_module = "extism:host/user")]
extern "C" {
    #[link_name = "aggrega_cli_write_stdout"]
    fn aggrega_cli_write_stdout(ptr: u64, len: u64);
    #[link_name = "aggrega_cli_write_stderr"]
    fn aggrega_cli_write_stderr(ptr: u64, len: u64);
}

/// The JSON envelope `aggrega_fetch_external_input` returns. Exactly
/// one field is populated: `content` carries the fetched bytes in
/// standard base64 encoding on success; `error` carries a failure
/// message naming the URL and the remedy.
#[derive(serde::Deserialize)]
struct FetchEnvelope {
    content: Option<String>,
    error: Option<String>,
}

/// Emit a log line to the Aggrega host. `level` is the severity
/// (0 = trace, 1 = debug, 2 = info, 3 = warn, 4 = error per the
/// project's log-level integer convention). `message` is the log
/// text.
///
/// When the host call fails, surface the failure via Extism's own
/// log channel at warn level rather than silently discarding. If the
/// fallback log call also fails (host log channel unavailable), the
/// guest has no further reporting path — the failure is intentionally
/// dropped at that point.
pub fn log(level: u64, message: &str) {
    if let Err(e) = unsafe { aggrega_log(level, message.to_string()) } {
        warn!("aggrega_log host call failed: {e:?}");
    }
}

/// Emit a structured warning the host's build-time validator surfaces
/// via the warning channel. `code` is a short identifier the
/// validator categorises by; `message` is the warning text; `path` is
/// an optional JSON-Pointer-style location into the authored entity
/// (e.g., "components.payments-api.values.memory"). Pass an empty
/// string for `path` when the warning is not attached to a specific
/// authored location.
///
/// Error-path treatment mirrors [`log`]: surface via Extism's own log
/// channel on host-call failure rather than silently discarding.
pub fn emit_warning(code: &str, message: &str, path: &str) {
    if let Err(e) =
        unsafe { aggrega_emit_warning(code.to_string(), message.to_string(), path.to_string()) }
    {
        warn!("aggrega_emit_warning host call failed: {e:?}");
    }
}

/// Fetch the bytes of an external-input URL through the Aggrega host's
/// acquisition channel. The host serves the URL under its
/// external-input pin discipline: an already-pinned URL is served from
/// the content-addressed store; an unpinned URL is fetched and pinned
/// when acquisition is enabled, and refused with a clear message when
/// the closure is frozen.
///
/// Failure is in-band: the host returns an error envelope rather than
/// trapping, and this wrapper surfaces it as the returned error — its
/// message names the URL and the remedy. Plugin code should treat a
/// fetch error as a render failure to report, not a condition to mask.
pub fn fetch_external_input(url: &str) -> Result<Vec<u8>, Error> {
    let raw = unsafe { aggrega_fetch_external_input(url.to_string())? };
    let envelope: FetchEnvelope = serde_json::from_str(&raw)
        .map_err(|e| Error::msg(format!("aggrega_fetch_external_input: decode envelope: {e}")))?;
    if let Some(message) = envelope.error {
        return Err(Error::msg(message));
    }
    match envelope.content {
        Some(content) => base64::engine::general_purpose::STANDARD
            .decode(content.as_bytes())
            .map_err(|e| Error::msg(format!("aggrega_fetch_external_input: decode content: {e}"))),
        None => Err(Error::msg(
            "aggrega_fetch_external_input: envelope carries neither content nor error",
        )),
    }
}

/// Ask the Aggrega host to pack a selection from the acquired source and
/// return the packed files. `op` names the shape of the selection ("file",
/// "dir", or "tree") and `path` names it relative to the source root (the
/// empty string names the root itself). The host resolves the selection under
/// the source root, refusing any path that escapes above it, and returns the
/// packed bytes as a source projection.
///
/// This is the guest side of the host-listing source model: the host hands the
/// guest a name-only listing, the guest applies its own resolution logic, and
/// this call is the guest's request that the host read and pack the chosen
/// selection — the only point at which source bytes cross into the guest.
///
/// Failure is in-band: the host returns an error envelope rather than trapping,
/// and this wrapper surfaces it as the returned error. Plugin code should treat
/// a pack failure as a render failure to report, not a condition to mask.
pub fn pack_source(op: &str, path: &str) -> Result<SourceProjection, Error> {
    let request = serde_json::to_string(&SourcePackRequest {
        op: op.to_string(),
        path: path.to_string(),
    })
    .map_err(|e| Error::msg(format!("aggrega_pack_source: encode request: {e}")))?;
    let raw = unsafe { aggrega_pack_source(request)? };
    let response: SourcePackResponse = serde_json::from_str(&raw)
        .map_err(|e| Error::msg(format!("aggrega_pack_source: decode response: {e}")))?;
    if !response.error.is_empty() {
        return Err(Error::msg(response.error));
    }
    match response.source_projection {
        Some(projection) => Ok(*projection),
        None => Err(Error::msg(
            "aggrega_pack_source: response carries neither a source projection nor an error",
        )),
    }
}

/// Ask the Aggrega host to read a single file from the acquired source and
/// return its bytes. `path` names the file relative to the source root. The
/// host resolves the path under the source root, refusing any path that escapes
/// above it, and returns the file's raw bytes.
///
/// It is the content-based-selection primitive: a guest whose resolution logic
/// must inspect a file's content before choosing what to pack reads that file
/// through this call, then issues its [`pack_source`] request.
///
/// Failure is in-band, matching [`pack_source`]: the host returns an error
/// envelope rather than trapping, and this wrapper surfaces it as the returned
/// error.
pub fn read_source_file(path: &str) -> Result<Vec<u8>, Error> {
    let request = serde_json::to_string(&SourceFileRequest {
        path: path.to_string(),
    })
    .map_err(|e| Error::msg(format!("aggrega_read_source_file: encode request: {e}")))?;
    let raw = unsafe { aggrega_read_source_file(request)? };
    let response: SourceFileResponse = serde_json::from_str(&raw)
        .map_err(|e| Error::msg(format!("aggrega_read_source_file: decode response: {e}")))?;
    if !response.error.is_empty() {
        return Err(Error::msg(response.error));
    }
    base64::engine::general_purpose::STANDARD
        .decode(response.content.as_bytes())
        .map_err(|e| Error::msg(format!("aggrega_read_source_file: decode content: {e}")))
}

/// Ask the Aggrega host to obtain a source's canonical structured tree by running
/// a parser for the given format through the host, and return the tree. `format`
/// is the syntactic format of the source (for example "json"); `source` is the
/// raw source bytes; `options` is an optional opaque options document the parser
/// interprets. The host resolves the parser for the format, runs it, and returns
/// the tree it produced.
///
/// This is the guest side of host-mediated producer consumption: a consumer
/// plugin obtains a producer's output through the host rather than invoking a
/// producer guest itself. The host bounds it to a single level — a producer
/// reached this way that calls `parse_source` again is denied — so a consumer
/// calls it once per source and may consume many sources in sequence.
///
/// Failure is in-band: the host returns an error envelope rather than trapping,
/// and this wrapper surfaces it as the returned error — a missing parser for the
/// format, a parse failure, or a denied re-entrant call all arrive as the error's
/// message. Plugin code should treat a parse-source error as a failure to report,
/// not a condition to mask.
pub fn parse_source(
    format: &str,
    source: &[u8],
    options: Option<Box<RawValue>>,
) -> Result<Box<RawValue>, Error> {
    let request = serde_json::to_string(&ParseInput {
        format: format.to_string(),
        bytes: base64::engine::general_purpose::STANDARD.encode(source),
        options,
    })
    .map_err(|e| Error::msg(format!("aggrega_parse_source: encode request: {e}")))?;
    let raw = unsafe { aggrega_parse_source(request)? };
    let response: ParseSourceResponse = serde_json::from_str(&raw)
        .map_err(|e| Error::msg(format!("aggrega_parse_source: decode response: {e}")))?;
    if !response.error.is_empty() {
        return Err(Error::msg(response.error));
    }
    match response.tree {
        Some(tree) => Ok(tree),
        None => Err(Error::msg(
            "aggrega_parse_source: response carries neither a tree nor an error",
        )),
    }
}

/// Stream a chunk of bytes to the Aggrega host's standard-output stream for the
/// running CLI command. The host copies the bytes out of guest memory and
/// writes them, incrementally, to the terminal's stdout — there is no single
/// output payload, so a plugin calls `write_stdout` as many times as it needs
/// while it runs. This binding is granted only when the guest runs as a CLI
/// plugin; a build-time load never binds it.
///
/// When the allocation of the outgoing bytes fails, surface the failure via
/// Extism's own log channel at warn level rather than panicking, matching the
/// error-path treatment of [`log`].
pub fn write_stdout(data: &[u8]) {
    match Memory::from_bytes(data) {
        Ok(mem) => {
            unsafe { aggrega_cli_write_stdout(mem.offset(), mem.len() as u64) };
            mem.free();
        }
        Err(e) => warn!("aggrega_cli_write_stdout: allocation failed: {e:?}"),
    }
}

/// Stream a chunk of bytes to the Aggrega host's standard-error stream for the
/// running CLI command. It is the stderr sibling of [`write_stdout`] and
/// carries the same runtime-only grant.
pub fn write_stderr(data: &[u8]) {
    match Memory::from_bytes(data) {
        Ok(mem) => {
            unsafe { aggrega_cli_write_stderr(mem.offset(), mem.len() as u64) };
            mem.free();
        }
        Err(e) => warn!("aggrega_cli_write_stderr: allocation failed: {e:?}"),
    }
}

/// Read the `cli_run` input payload the host hands a CLI plugin on each
/// invocation and return the argument vector — the tokens that follow the
/// resolved subcommand name on the command line, in invocation order. The
/// plugin parses these tokens as its own flags and positional arguments,
/// exactly as a standalone command-line program reads the arguments after its
/// name.
pub fn cli_args() -> Result<Vec<String>, Error> {
    let raw = extism_pdk::input::<String>()?;
    let parsed: CLIRunInput = serde_json::from_str(&raw)
        .map_err(|e| Error::msg(format!("aggrega_cli_run: decode CLIRunInput: {e}")))?;
    Ok(parsed.args)
}
