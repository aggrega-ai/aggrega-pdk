// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

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
    CLIRunInput, FetchExternalInputResponse, ParseInput, ParseSourceResponse, SourceFileRequest,
    SourceFileResponse, SourcePackRequest, SourcePackResponse, SourceProjection,
};
use base64::Engine as _;
use extism_pdk::{host_fn, warn, Error, Memory};
use serde_json::value::RawValue;

#[host_fn("extism:host/user")]
extern "ExtismHost" {
    fn aggrega_log(level: u64, message: String);
    fn aggrega_emit_warning(code: String, message: String, path: String);
    // The URL is a bare string rather than a payload document, so it stays
    // UTF-8 text; the reply is a payload document and carries canonical CBOR,
    // so it is declared as an opaque byte run for the reason given below.
    fn aggrega_fetch_external_input(url: String) -> Vec<u8>;
    // Both slots on each of the three payload-carrying callbacks below carry
    // canonical CBOR, so each is declared as an opaque byte run rather than as
    // UTF-8 text. The declared type is the contract here, not a formatting
    // choice: `String` would put the request through a text conversion that
    // binary bytes do not survive.
    fn aggrega_pack_source(request: Vec<u8>) -> Vec<u8>;
    fn aggrega_read_source_file(request: Vec<u8>) -> Vec<u8>;
    fn aggrega_parse_source(request: Vec<u8>) -> Vec<u8>;
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

/// The error a host-function wrapper returns when the host reports an in-band
/// failure: the machine-matchable `code` beside the free-form `message`. The
/// Display impl is the message, so a consumer that only logs or wraps the error
/// is unaffected by the added code; a consumer that wants to branch downcasts
/// and compares `code` against a `CODE_*` constant:
///
/// ```ignore
/// if let Some(he) = err.downcast_ref::<HostError>() {
///     if he.code == aggrega_pdk_rust::CODE_SOURCE_ESCAPE { /* handle escape */ }
/// }
/// ```
///
/// Absent- and unknown-code tolerance is built in: an older host that sends no
/// code leaves `code` empty, and a newer host that sends a code this build
/// predates leaves `code` set to that unrecognized value — either way a compare
/// against a known constant does not match and the consumer falls back to the
/// message, with no panic and no dropped error.
#[derive(Debug)]
pub struct HostError {
    pub code: String,
    pub message: String,
}

impl std::fmt::Display for HostError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(&self.message)
    }
}

impl std::error::Error for HostError {}

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
/// The URL travels as UTF-8 text, because it is a bare string rather than a
/// payload document. The reply carries canonical CBOR, success and failure
/// alike, with the fetched bytes as a raw byte string: a payload that is not
/// valid UTF-8 arrives intact, and a zero-length payload returns an empty
/// vector rather than an error. Plugin code calling this wrapper needs no CBOR
/// library and hand-parses nothing.
///
/// Failure is in-band: the host returns an error envelope rather than
/// trapping, and this wrapper surfaces it as the returned error — a
/// [`HostError`] whose message names the URL and the remedy and whose code is
/// one of the `CODE_FETCH_*` constants. Plugin code should treat a fetch error
/// as a render failure to report, not a condition to mask.
pub fn fetch_external_input(url: &str) -> Result<Vec<u8>, Error> {
    let raw = unsafe { aggrega_fetch_external_input(url.to_string())? };
    decode_fetch_external_input_reply(&raw)
}

/// Reads one `aggrega_fetch_external_input` reply and returns the fetched
/// bytes, or the failure it reports.
///
/// The reply slot carries canonical CBOR in both the success and the failure
/// case, so one decoder reads both. A reply carrying an error message or an
/// error code is a failure and surfaces as a [`HostError`]. Any other reply is
/// a success, and its content is the payload: the field is omitted when empty,
/// so a zero-length payload arrives as a reply with no field at all and yields
/// an empty vector. Reading the failure from the failure fields, rather than
/// from the content key's absence, is what keeps that empty payload
/// distinguishable from a failure.
///
/// The payload type's `content` field holds base64 in this crate, matching
/// the textual encoding's own field type, and the generated decoder re-encodes
/// the raw byte run to base64 on the way in — so the base64 step here is the
/// one that yields the fetched bytes.
///
/// Split out from [`fetch_external_input`] so the wire is exercisable where
/// the call is not: the host import only a running host resolves, so a test
/// off Wasm can reach this and cannot reach the call.
fn decode_fetch_external_input_reply(reply: &[u8]) -> Result<Vec<u8>, Error> {
    let response = FetchExternalInputResponse::decode_cbor(reply)
        .map_err(|e| Error::msg(format!("aggrega_fetch_external_input: decode response: {e}")))?;
    if !response.error.is_empty() || !response.hostfunc_error_code.is_empty() {
        let message = if response.error.is_empty() {
            format!(
                "aggrega_fetch_external_input: the host reported failure {}",
                response.hostfunc_error_code
            )
        } else {
            response.error
        };
        return Err(Error::new(HostError {
            code: response.hostfunc_error_code,
            message,
        }));
    }
    base64::engine::general_purpose::STANDARD
        .decode(response.content.as_bytes())
        .map_err(|e| Error::msg(format!("aggrega_fetch_external_input: decode content: {e}")))
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
/// Both slots on this call carry canonical CBOR, so the request travels as an
/// opaque byte run rather than as UTF-8 text and the reply is read the same way.
/// Plugin code calling this wrapper needs no CBOR library and hand-parses
/// nothing.
///
/// Failure is in-band: the host returns an error envelope rather than trapping,
/// and this wrapper surfaces it as the returned error. Plugin code should treat
/// a pack failure as a render failure to report, not a condition to mask.
pub fn pack_source(op: &str, path: &str) -> Result<SourceProjection, Error> {
    let request = encode_pack_source_request(op, path)?;
    let raw = unsafe { aggrega_pack_source(request)? };
    decode_pack_source_reply(&raw)
}

/// Builds the request bytes for one `aggrega_pack_source` call.
///
/// The request slot carries canonical CBOR, so the bytes are an opaque byte run
/// rather than UTF-8 text, and they come from the payload type's own generated
/// encoder rather than from anything assembled here.
///
/// Split out from [`pack_source`] so the wire is exercisable where the call is
/// not: the host import only a running host resolves, so a test off Wasm can
/// reach this and cannot reach the call.
fn encode_pack_source_request(op: &str, path: &str) -> Result<Vec<u8>, Error> {
    SourcePackRequest {
        op: op.to_string(),
        path: path.to_string(),
    }
    .encode_cbor()
    .map_err(|e| Error::msg(format!("aggrega_pack_source: encode request: {e}")))
}

/// Reads one `aggrega_pack_source` reply and returns the packed source it
/// carries, or the failure it reports.
///
/// The reply slot carries canonical CBOR in both the success and the failure
/// case: a guest reads one slot and cannot know before decoding which of the two
/// it holds, so one decoder reads both. A reported failure surfaces as a
/// [`HostError`] carrying the host's machine-matchable code beside its message; a
/// reply carrying neither a projection nor a failure is itself an error, so an
/// empty reply cannot pass as an empty pack.
fn decode_pack_source_reply(reply: &[u8]) -> Result<SourceProjection, Error> {
    let response = SourcePackResponse::decode_cbor(reply)
        .map_err(|e| Error::msg(format!("aggrega_pack_source: decode response: {e}")))?;
    if !response.error.is_empty() {
        return Err(Error::new(HostError {
            code: response.hostfunc_error_code,
            message: response.error,
        }));
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
/// Both slots carry canonical CBOR, on the same terms as [`pack_source`].
///
/// Failure is in-band, matching [`pack_source`]: the host returns an error
/// envelope rather than trapping, and this wrapper surfaces it as the returned
/// error.
pub fn read_source_file(path: &str) -> Result<Vec<u8>, Error> {
    let request = encode_read_source_file_request(path)?;
    let raw = unsafe { aggrega_read_source_file(request)? };
    decode_read_source_file_reply(&raw)
}

/// Builds the request bytes for one `aggrega_read_source_file` call.
///
/// The request slot carries canonical CBOR, on the same terms as
/// [`encode_pack_source_request`], and it is split out from
/// [`read_source_file`] for the same reason.
fn encode_read_source_file_request(path: &str) -> Result<Vec<u8>, Error> {
    SourceFileRequest {
        path: path.to_string(),
    }
    .encode_cbor()
    .map_err(|e| Error::msg(format!("aggrega_read_source_file: encode request: {e}")))
}

/// Reads one `aggrega_read_source_file` reply and returns the file's bytes, or
/// the failure it reports.
///
/// The payload type's `content` field holds base64 in this crate, matching the
/// textual encoding's own field type, and the generated decoder re-encodes the
/// raw byte run to base64 on the way in — so the base64 step here is still the
/// one that yields the file's actual bytes.
///
/// Unlike [`decode_pack_source_reply`] this does not treat an absent payload as
/// an error: an empty file is a legitimate read, and its reply carries no content
/// key at all because the field is omitted when empty. So a reply with no failure
/// yields an empty vector, which is the file's actual content.
fn decode_read_source_file_reply(reply: &[u8]) -> Result<Vec<u8>, Error> {
    let response = SourceFileResponse::decode_cbor(reply)
        .map_err(|e| Error::msg(format!("aggrega_read_source_file: decode response: {e}")))?;
    if !response.error.is_empty() {
        return Err(Error::new(HostError {
            code: response.hostfunc_error_code,
            message: response.error,
        }));
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
    let request = encode_parse_source_request(format, source, options)?;
    let raw = unsafe { aggrega_parse_source(request)? };
    decode_parse_source_reply(&raw)
}

/// Builds the request bytes for one `aggrega_parse_source` call.
///
/// The request slot carries canonical CBOR, so the bytes are an opaque byte run
/// rather than UTF-8 text, and they come from the payload type's own generated
/// encoder rather than from anything assembled here. The payload's `bytes` field
/// holds base64 in the textual encoding, and the generated encoder decodes that
/// back to the raw run before writing it, so the source bytes reach the host once
/// rather than twice-encoded.
///
/// Split out from [`parse_source`] so the wire is exercisable where the call is
/// not: the host import only a running host resolves, so a test off Wasm can
/// reach this and cannot reach the call.
fn encode_parse_source_request(
    format: &str,
    source: &[u8],
    options: Option<Box<RawValue>>,
) -> Result<Vec<u8>, Error> {
    ParseInput {
        format: format.to_string(),
        bytes: base64::engine::general_purpose::STANDARD.encode(source),
        options,
    }
    .encode_cbor()
    .map_err(|e| Error::msg(format!("aggrega_parse_source: encode request: {e}")))
}

/// Reads one `aggrega_parse_source` reply and returns the canonical tree it
/// carries, or the failure it reports.
///
/// The reply slot carries canonical CBOR in both the success and the failure
/// case: a guest reads one slot and cannot know before decoding which of the two
/// it holds, so one decoder reads both. A reported failure surfaces as a
/// [`HostError`] carrying the host's machine-matchable code beside its message; a
/// reply carrying neither a tree nor a failure is itself an error, so an empty
/// reply cannot pass as an empty tree.
fn decode_parse_source_reply(reply: &[u8]) -> Result<Box<RawValue>, Error> {
    let response = ParseSourceResponse::decode_cbor(reply)
        .map_err(|e| Error::msg(format!("aggrega_parse_source: decode reply: {e}")))?;
    if !response.error.is_empty() {
        return Err(Error::new(HostError {
            code: response.hostfunc_error_code,
            message: response.error,
        }));
    }
    match response.tree {
        Some(tree) => Ok(tree),
        None => Err(Error::msg(
            "aggrega_parse_source: reply carries neither a tree nor an error",
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

#[cfg(test)]
mod tests {
    //! Wire tests for the callbacks whose replies carry the binary encoding —
    //! `aggrega_pack_source`, `aggrega_read_source_file`,
    //! `aggrega_parse_source` and `aggrega_fetch_external_input` — driven against
    //! the shared payload vectors the ABI crate publishes rather than against
    //! bytes produced here. That is what makes them discriminating: a kit that
    //! built its request with the wrong encoder, or read its reply with the wrong
    //! decoder, produces bytes that do not match a vector an independent
    //! instrument has already anchored — whereas a kit checked only against its
    //! own output round-trips cleanly however wrong both halves are, because the
    //! same mistake is on both sides.
    //!
    //! The tests exercise the wire functions and never the host calls themselves:
    //! each call's import is resolved only by a running host, so a test off Wasm
    //! that reached one would not link.

    use super::{
        decode_fetch_external_input_reply, decode_pack_source_reply, decode_parse_source_reply,
        decode_read_source_file_reply, encode_pack_source_request, encode_parse_source_request,
        encode_read_source_file_request, HostError,
    };
    use base64::Engine as _;
    use std::path::PathBuf;

    /// Resolves the directory holding the shared payload vectors. Resolution
    /// only; whether it can be read is decided in one place, by `read_vector`.
    fn vector_dir() -> PathBuf {
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("..")
            .join("..")
            .join("aggrega-abi")
            .join("testdata")
    }

    /// Reads one shared vector file. A run that could not read the vectors has
    /// verified nothing, so this refuses with a diagnostic naming the path rather
    /// than skipping quietly.
    fn read_vector(name: &str) -> String {
        let path = vector_dir().join(name);
        std::fs::read_to_string(&path).unwrap_or_else(|e| {
            panic!(
                "shared payload vector {name} is not readable at {}: {e}; the vectors are read from the testdata directory of the aggrega-abi repository, which must be checked out beside this one",
                path.display()
            )
        })
    }

    /// Reads one shared canonical-CBOR vector and decodes its hex.
    fn read_hex_vector(name: &str) -> Vec<u8> {
        let raw = read_vector(name);
        let raw = raw.trim();
        assert!(
            raw.len() % 2 == 0,
            "shared vector {name} does not hold an even number of hex digits"
        );
        (0..raw.len())
            .step_by(2)
            .map(|i| {
                u8::from_str_radix(&raw[i..i + 2], 16).unwrap_or_else(|e| {
                    panic!("shared vector {name} does not hold decodable hex: {e}")
                })
            })
            .collect()
    }

    /// The request the kit builds is byte-identical to the shared `ParseInput`
    /// vector, for the very field values that vector's companion document
    /// declares. A kit still building a text request fails here, because a text
    /// document and the canonical binary encoding of the same value are different
    /// bytes.
    #[test]
    fn encode_request_matches_shared_vector() {
        // The companion document carries the field values in the textual
        // encoding, where the source bytes are base64. Read them from there
        // rather than restating them, so the two vectors cannot drift apart from
        // this test.
        let golden: serde_json::Value =
            serde_json::from_str(&read_vector("ParseInput.golden.json"))
                .expect("ParseInput.golden.json is readable as JSON");
        let format = golden["format"].as_str().expect("golden carries a format");
        let source = base64::engine::general_purpose::STANDARD
            .decode(golden["bytes"].as_str().expect("golden carries bytes"))
            .expect("golden source bytes are base64");
        let options = serde_json::value::to_raw_value(&golden["options"])
            .expect("golden options re-serialise");

        let got = encode_parse_source_request(format, &source, Some(options))
            .expect("encode_parse_source_request");
        let want = read_hex_vector("ParseInput.cbor.hex");
        assert_eq!(
            hex(&got),
            hex(&want),
            "request bytes do not match the shared ParseInput vector"
        );
    }

    /// The kit reads a reported failure out of the shared `ParseSourceResponse`
    /// vector and surfaces it as a `HostError` carrying both the
    /// machine-matchable code and the message.
    #[test]
    fn decode_reply_failure_from_shared_vector() {
        let golden: serde_json::Value =
            serde_json::from_str(&read_vector("ParseSourceResponse.golden.json"))
                .expect("ParseSourceResponse.golden.json is readable as JSON");
        let want_message = golden["error"].as_str().expect("golden carries an error");
        let want_code = golden["hostfunc_error_code"]
            .as_str()
            .expect("golden carries a hostfunc_error_code");

        let err = decode_parse_source_reply(&read_hex_vector("ParseSourceResponse.cbor.hex"))
            .expect_err("a failure reply must not decode as a success");
        let host_err = err
            .downcast_ref::<HostError>()
            .expect("the failure surfaces as a HostError");
        assert_eq!(host_err.code, want_code);
        assert_eq!(host_err.message, want_message);
    }

    /// The kit reads the success shape too — the one production guests take, and
    /// the only one carrying the tree.
    ///
    /// Both shapes are asserted because each hides the other's defect: a decoder
    /// that only reads the failure fields passes any test built from failure
    /// replies alone, and one that only reads the tree passes any test built from
    /// success replies alone.
    ///
    /// The vector is authored from the canonical encoding's own rules rather than
    /// captured, and it is short enough to read off:
    ///
    /// ```text
    /// a1                      map, 1 pair
    /// 64 74 72 65 65          text, 4 bytes: "tree"
    /// 47 7b 22 61 22 3a 31 7d byte run, 7 bytes: {"a":1}
    /// ```
    ///
    /// One pair is what a tree-only envelope carries, since the two failure
    /// fields are omitted when empty; and the tree travels as a byte run holding
    /// its own document verbatim, which is why an over- or under-declared pair
    /// count would move the head byte and fail here rather than pass silently.
    #[test]
    fn decode_reply_success_shape() {
        let reply: Vec<u8> = vec![
            0xa1, 0x64, 0x74, 0x72, 0x65, 0x65, 0x47, 0x7b, 0x22, 0x61, 0x22, 0x3a, 0x31, 0x7d,
        ];
        let tree = decode_parse_source_reply(&reply).expect("decode_parse_source_reply");
        assert_eq!(tree.get(), r#"{"a":1}"#);
    }

    /// The reply decoder is not silently tolerant of a textual reply. Without
    /// this, a host left on the other encoding would be reported as some
    /// downstream shape error rather than as an undecodable reply, and the two are
    /// not the same diagnosis.
    #[test]
    fn decode_reply_refuses_the_other_encoding() {
        let textual = br#"{"tree":{"a":1}}"#;
        assert!(
            decode_parse_source_reply(textual).is_err(),
            "a textual reply decoded without error; the reply slot carries the binary encoding only"
        );
    }

    /// An envelope carrying neither a tree nor a failure is reported rather than
    /// passing as an empty tree. The distinction matters because an empty tree is
    /// itself a legitimate outcome, so a lenient decode would make "the host said
    /// nothing" and "the parser produced nothing" indistinguishable.
    #[test]
    fn decode_reply_empty_envelope_is_an_error() {
        // a0: map, 0 pairs — every field omitted.
        assert!(decode_parse_source_reply(&[0xa0]).is_err());
    }

    /// The pack-source request the kit builds is byte-identical to the shared
    /// `SourcePackRequest` vector, for the very field values that vector's
    /// companion document declares. A kit still building a text request fails
    /// here, because a text document and the canonical binary encoding of the same
    /// value are different bytes.
    #[test]
    fn encode_pack_source_request_matches_shared_vector() {
        let golden: serde_json::Value =
            serde_json::from_str(&read_vector("SourcePackRequest.golden.json"))
                .expect("SourcePackRequest.golden.json is readable as JSON");
        let op = golden["op"].as_str().expect("golden carries an op");
        let path = golden["path"].as_str().expect("golden carries a path");

        let got = encode_pack_source_request(op, path).expect("encode_pack_source_request");
        let want = read_hex_vector("SourcePackRequest.cbor.hex");
        assert_eq!(
            hex(&got),
            hex(&want),
            "request bytes do not match the shared SourcePackRequest vector"
        );
    }

    /// The content-read sibling of the pack-source request assertion.
    #[test]
    fn encode_read_source_file_request_matches_shared_vector() {
        let golden: serde_json::Value =
            serde_json::from_str(&read_vector("SourceFileRequest.golden.json"))
                .expect("SourceFileRequest.golden.json is readable as JSON");
        let path = golden["path"].as_str().expect("golden carries a path");

        let got = encode_read_source_file_request(path).expect("encode_read_source_file_request");
        let want = read_hex_vector("SourceFileRequest.cbor.hex");
        assert_eq!(
            hex(&got),
            hex(&want),
            "request bytes do not match the shared SourceFileRequest vector"
        );
    }

    /// The kit reads the pack-source success shape out of the shared vector — the
    /// shape every production provider guest takes, and the only one carrying the
    /// packed files.
    ///
    /// This vector carries the projection with both failure fields omitted; the
    /// failure test below carries the failure fields with the projection omitted.
    /// Both directions are asserted because each hides the other's defect: a
    /// decoder that only reads the failure fields passes any test built from
    /// failure replies alone, and one that only reads the projection passes any
    /// test built from success replies alone.
    #[test]
    fn decode_pack_source_reply_success_from_shared_vector() {
        let golden: serde_json::Value =
            serde_json::from_str(&read_vector("SourcePackResponse.golden.json"))
                .expect("SourcePackResponse.golden.json is readable as JSON");
        let want_files = golden["source_projection"]["files"]
            .as_array()
            .expect("golden carries a files array");

        let projection = decode_pack_source_reply(&read_hex_vector("SourcePackResponse.cbor.hex"))
            .expect("decode_pack_source_reply on the success shape");
        assert_eq!(
            projection.files.len(),
            want_files.len(),
            "decoded file count does not match the vector"
        );
        for (i, want) in want_files.iter().enumerate() {
            let want_path = want["path"].as_str().expect("vector file carries a path");
            // The companion document carries each file's content base64, which is
            // the shape the payload type's own field holds in this crate.
            let want_content = want["content"]
                .as_str()
                .expect("vector file carries content");
            assert_eq!(projection.files[i].path, want_path, "file {i} path");
            assert_eq!(
                projection.files[i].content, want_content,
                "file {i} content"
            );
        }
    }

    /// The kit reads a reported pack-source failure out of the shared error
    /// envelope vector and surfaces it as a `HostError` carrying both the
    /// machine-matchable code and the message.
    #[test]
    fn decode_pack_source_reply_failure_from_shared_vector() {
        let golden: serde_json::Value =
            serde_json::from_str(&read_vector("ErrorEnvelopeSourcePackResponse.golden.json"))
                .expect("ErrorEnvelopeSourcePackResponse.golden.json is readable as JSON");
        let want_message = golden["error"].as_str().expect("golden carries an error");
        let want_code = golden["hostfunc_error_code"]
            .as_str()
            .expect("golden carries a hostfunc_error_code");

        let err =
            decode_pack_source_reply(&read_hex_vector("ErrorEnvelopeSourcePackResponse.cbor.hex"))
                .expect_err("a failure reply must not decode as a success");
        let host_err = err
            .downcast_ref::<HostError>()
            .expect("the failure surfaces as a HostError");
        assert_eq!(host_err.code, want_code);
        assert_eq!(host_err.message, want_message);
    }

    /// The kit reads the file's bytes out of the shared `SourceFileResponse`
    /// vector, whose companion document carries them base64.
    #[test]
    fn decode_read_source_file_reply_success_from_shared_vector() {
        let golden: serde_json::Value =
            serde_json::from_str(&read_vector("SourceFileResponse.golden.json"))
                .expect("SourceFileResponse.golden.json is readable as JSON");
        let want = base64::engine::general_purpose::STANDARD
            .decode(golden["content"].as_str().expect("golden carries content"))
            .expect("golden content is base64");

        let got = decode_read_source_file_reply(&read_hex_vector("SourceFileResponse.cbor.hex"))
            .expect("decode_read_source_file_reply on the success shape");
        assert_eq!(hex(&got), hex(&want), "content does not match the vector");
    }

    /// The content-read failure shape surfaces as a `HostError` — the other
    /// direction of the omission property from the success vector above.
    #[test]
    fn decode_read_source_file_reply_failure_from_shared_vector() {
        let golden: serde_json::Value =
            serde_json::from_str(&read_vector("ErrorEnvelopeSourceFileResponse.golden.json"))
                .expect("ErrorEnvelopeSourceFileResponse.golden.json is readable as JSON");
        let want_message = golden["error"].as_str().expect("golden carries an error");
        let want_code = golden["hostfunc_error_code"]
            .as_str()
            .expect("golden carries a hostfunc_error_code");

        let err = decode_read_source_file_reply(&read_hex_vector(
            "ErrorEnvelopeSourceFileResponse.cbor.hex",
        ))
        .expect_err("a failure reply must not decode as a success");
        let host_err = err
            .downcast_ref::<HostError>()
            .expect("the failure surfaces as a HostError");
        assert_eq!(host_err.code, want_code);
        assert_eq!(host_err.message, want_message);
    }

    /// Neither source reply decoder is silently tolerant of a textual reply.
    /// Without this, a host left on the other encoding would be reported as some
    /// downstream shape error rather than as an undecodable reply, and the two are
    /// not the same diagnosis.
    #[test]
    fn decode_source_replies_refuse_the_other_encoding() {
        assert!(
            decode_pack_source_reply(br#"{"source_projection":{"files":[]}}"#).is_err(),
            "a textual pack-source reply decoded without error"
        );
        assert!(
            decode_read_source_file_reply(br#"{"content":"aGk="}"#).is_err(),
            "a textual read-source-file reply decoded without error"
        );
    }

    /// A pack-source envelope carrying neither a projection nor a failure is
    /// reported rather than passing as an empty pack, so "the host said nothing"
    /// and "the selection matched nothing" stay distinguishable.
    #[test]
    fn decode_pack_source_reply_empty_envelope_is_an_error() {
        // a0: map, 0 pairs — every field omitted.
        assert!(decode_pack_source_reply(&[0xa0]).is_err());
    }

    /// The deliberate asymmetry with the pack-source decoder: an empty content
    /// field is a legitimate read of an empty file, and the field is omitted when
    /// empty, so a reply with no failure and no content yields empty bytes rather
    /// than an error.
    ///
    /// Worth pinning rather than inferring: making this an error would refuse
    /// every empty file in an acquired source, and making the pack-source case
    /// lenient would let a silent host produce an empty pack.
    #[test]
    fn decode_read_source_file_reply_empty_envelope_is_the_empty_file() {
        // a0: map, 0 pairs — every field omitted.
        let got = decode_read_source_file_reply(&[0xa0])
            .expect("an empty-content reply reads as the empty file");
        assert!(got.is_empty(), "content = {got:?}; want the empty file");
    }

    /// The map head byte of each shared source vector, read from the canonical
    /// encoding's own rules rather than from the encoder that wrote them. A head
    /// disagreeing with its body round-trips cleanly through the generator's own
    /// two halves, so reading the head is what a same-generator comparison cannot
    /// do.
    ///
    /// The heads assert the omission property in both directions at once: a
    /// success envelope carries only its payload key, and a failure envelope
    /// carries only the two failure keys.
    #[test]
    fn source_vector_heads() {
        for (vector, want, why) in [
            (
                "SourcePackRequest.cbor.hex",
                0xa2u8,
                "two required fields: op and path",
            ),
            (
                "SourceFileRequest.cbor.hex",
                0xa1u8,
                "one required field: path",
            ),
            (
                "SourcePackResponse.cbor.hex",
                0xa1u8,
                "the projection alone; both failure fields omitted",
            ),
            (
                "SourceFileResponse.cbor.hex",
                0xa1u8,
                "the content alone; both failure fields omitted",
            ),
            (
                "ErrorEnvelopeSourcePackResponse.cbor.hex",
                0xa2u8,
                "message and code; the projection omitted",
            ),
            (
                "ErrorEnvelopeSourceFileResponse.cbor.hex",
                0xa2u8,
                "message and code; the content omitted",
            ),
        ] {
            let b = read_hex_vector(vector);
            assert!(!b.is_empty(), "{vector} is empty");
            assert_eq!(b[0], want, "{vector} head byte ({why})");
        }
    }

    /// The kit reads the fetched bytes out of the shared success vector, whose
    /// companion document carries them base64.
    #[test]
    fn decode_fetch_external_input_reply_content_from_shared_vector() {
        let golden: serde_json::Value = serde_json::from_str(&read_vector(
            "FetchExternalInputResponseContent.golden.json",
        ))
        .expect("FetchExternalInputResponseContent.golden.json is readable as JSON");
        let want = base64::engine::general_purpose::STANDARD
            .decode(golden["content"].as_str().expect("golden carries content"))
            .expect("golden content is base64");

        let got = decode_fetch_external_input_reply(&read_hex_vector(
            "FetchExternalInputResponseContent.cbor.hex",
        ))
        .expect("decode_fetch_external_input_reply on the success shape");
        assert_eq!(hex(&got), hex(&want), "content does not match the vector");
    }

    /// A zero-length payload is a success yielding empty bytes. Its reply
    /// carries no field at all, because the content field is omitted when
    /// empty, so a reader that took an absent content key for a failure fails
    /// here.
    #[test]
    fn decode_fetch_external_input_reply_empty_content_from_shared_vector() {
        let got = decode_fetch_external_input_reply(&read_hex_vector(
            "FetchExternalInputResponseEmptyContent.cbor.hex",
        ))
        .expect("a zero-length payload reads as empty bytes, not as an error");
        assert!(got.is_empty(), "content = {got:?}; want empty bytes");
    }

    /// The fetch failure shape surfaces as a `HostError` carrying both the
    /// machine-matchable code and the message.
    #[test]
    fn decode_fetch_external_input_reply_failure_from_shared_vector() {
        let golden: serde_json::Value =
            serde_json::from_str(&read_vector("FetchExternalInputResponse.golden.json"))
                .expect("FetchExternalInputResponse.golden.json is readable as JSON");
        let want_message = golden["error"].as_str().expect("golden carries an error");
        let want_code = golden["hostfunc_error_code"]
            .as_str()
            .expect("golden carries a hostfunc_error_code");

        let err = decode_fetch_external_input_reply(&read_hex_vector(
            "FetchExternalInputResponse.cbor.hex",
        ))
        .expect_err("a failure reply must not decode as a success");
        let host_err = err
            .downcast_ref::<HostError>()
            .expect("the failure surfaces as a HostError");
        assert_eq!(host_err.code, want_code);
        assert_eq!(host_err.message, want_message);
    }

    /// A reply carrying a failure code and no message is still a failure.
    /// Without this, such a reply would carry no content and no message, which
    /// is exactly the shape of the zero-length payload, and would read as a
    /// successful empty fetch.
    #[test]
    fn decode_fetch_external_input_reply_code_alone_is_a_failure() {
        let reply = aggrega_abi::FetchExternalInputResponse {
            hostfunc_error_code: aggrega_abi::CODE_FETCH_FAILED.to_string(),
            ..Default::default()
        }
        .encode_cbor()
        .expect("encode the code-only reply");
        let err = decode_fetch_external_input_reply(&reply)
            .expect_err("a code-only reply must not decode as a success");
        let host_err = err
            .downcast_ref::<HostError>()
            .expect("the failure surfaces as a HostError");
        assert_eq!(host_err.code, aggrega_abi::CODE_FETCH_FAILED);
        assert!(!host_err.message.is_empty(), "the failure carries a message");
    }

    /// The fetch reply decoder is not silently tolerant of a textual reply.
    #[test]
    fn decode_fetch_external_input_reply_refuses_the_other_encoding() {
        assert!(
            decode_fetch_external_input_reply(br#"{"content":"aGVsbG8="}"#).is_err(),
            "a textual fetch reply decoded without error; the reply slot carries the binary encoding only"
        );
    }

    /// The map head byte of each shared fetch vector, read from the canonical
    /// encoding's own rules, and the content vector's value head: a byte string
    /// (major type 2), which a text-string or base64 carry would not produce.
    #[test]
    fn fetch_vector_heads() {
        for (vector, want, why) in [
            (
                "FetchExternalInputResponseContent.cbor.hex",
                0xa1u8,
                "the content alone; both failure fields omitted",
            ),
            (
                "FetchExternalInputResponseEmptyContent.cbor.hex",
                0xa0u8,
                "no field at all: an empty content is omitted",
            ),
            (
                "FetchExternalInputResponse.cbor.hex",
                0xa2u8,
                "message and code; the content omitted",
            ),
        ] {
            let b = read_hex_vector(vector);
            assert!(!b.is_empty(), "{vector} is empty");
            assert_eq!(b[0], want, "{vector} head byte ({why})");
        }
        // a1, 67 "content", then the value head: 0x40 | length marks a byte string.
        let b = read_hex_vector("FetchExternalInputResponseContent.cbor.hex");
        assert!(
            b.len() >= 10 && b[9] & 0xe0 == 0x40,
            "content value head in {}; want a byte string (major type 2)",
            hex(&b)
        );
    }

    /// Renders bytes as hex so a mismatch assertion prints something readable
    /// rather than two long integer slices.
    fn hex(b: &[u8]) -> String {
        use std::fmt::Write as _;
        b.iter()
            .fold(String::with_capacity(b.len() * 2), |mut out, x| {
                // Writing into a String cannot fail, so there is no error to carry.
                let _ = write!(out, "{x:02x}");
                out
            })
    }
}
