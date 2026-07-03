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

use base64::Engine as _;
use extism_pdk::{host_fn, warn, Error};

#[host_fn("extism:host/user")]
extern "ExtismHost" {
    fn aggrega_log(level: u64, message: String);
    fn aggrega_emit_warning(code: String, message: String, path: String);
    fn aggrega_fetch_external_input(url: String) -> String;
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
