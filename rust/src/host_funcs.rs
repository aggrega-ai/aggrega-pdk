// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Bindings to the Aggrega host functions.
//!
//! The `#[host_fn]` macro from `extism-pdk` declares a Wasm import
//! interface: from the guest's perspective the functions are imported
//! from the host; the macro emits the i64-pointer marshalling so
//! plugin authors write Rust-typed signatures.
//!
//! Safe wrappers ([`log`], [`emit_warning`]) hide the `unsafe { ... }`
//! block from plugin authors. Plugin code calls `log(level, "message")`
//! directly; the wrapper handles the host-call invocation.

use extism_pdk::{host_fn, warn};

#[host_fn("extism:host/user")]
extern "ExtismHost" {
    fn aggrega_log(level: u64, message: String);
    fn aggrega_emit_warning(code: String, message: String, path: String);
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
