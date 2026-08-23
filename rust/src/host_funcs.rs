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
    fn aggrega_fetch_external_input(url: String) -> String;
    fn aggrega_pack_source(request: String) -> String;
    fn aggrega_read_source_file(request: String) -> String;
    // Both slots on this call carry canonical CBOR, so each is declared as an
    // opaque byte run rather than as UTF-8 text. The declared type is the
    // contract here, not a formatting choice: `String` would put the request
    // through a text conversion that binary bytes do not survive.
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
/// Failure is in-band: the host returns an error envelope rather than
/// trapping, and this wrapper surfaces it as the returned error — its
/// message names the URL and the remedy. Plugin code should treat a
/// fetch error as a render failure to report, not a condition to mask.
pub fn fetch_external_input(url: &str) -> Result<Vec<u8>, Error> {
    let raw = unsafe { aggrega_fetch_external_input(url.to_string())? };
    let envelope: FetchExternalInputResponse = serde_json::from_str(&raw)
        .map_err(|e| Error::msg(format!("aggrega_fetch_external_input: decode envelope: {e}")))?;
    if !envelope.error.is_empty() {
        return Err(Error::new(HostError {
            code: envelope.error_code,
            message: envelope.error,
        }));
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
        return Err(Error::new(HostError {
            code: response.error_code,
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
        return Err(Error::new(HostError {
            code: response.error_code,
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
            code: response.error_code,
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
    //! Wire tests for the `aggrega_parse_source` call, driven against the shared
    //! payload vectors the ABI crate publishes rather than against bytes produced
    //! here. That is what makes them discriminating: a kit that built its request
    //! with the wrong encoder, or read its reply with the wrong decoder, produces
    //! bytes that do not match a vector an independent instrument has already
    //! anchored — whereas a kit checked only against its own output round-trips
    //! cleanly however wrong both halves are, because the same mistake is on both
    //! sides.
    //!
    //! The tests exercise the two wire functions and never the host call itself:
    //! the call's import is resolved only by a running host, so a test off Wasm
    //! that reached it would not link.

    use super::{decode_parse_source_reply, encode_parse_source_request, HostError};
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
                "shared payload vector {name} is not readable at {}: {e}; run scripts/setup-workspace.sh from the umbrella checkout so the sibling ABI checkout is present",
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
        let want_code = golden["error_code"]
            .as_str()
            .expect("golden carries an error_code");

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
