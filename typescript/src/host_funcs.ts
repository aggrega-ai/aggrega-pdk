// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Bindings to the Aggrega host functions.
 *
 * The pattern follows the Extism JS PDK convention: a TypeScript
 * `declare module "extism:host"` block declares the import shape so
 * the compiler knows the functions exist under the `extism:host/user`
 * Wasm namespace, and the runtime obtains callable bindings via
 * `Host.getFunctions()` destructuring. The two layers are independent
 * — the type declaration is compile-time only; the actual invocation
 * goes through the destructured local bindings.
 *
 * Plugin authors call the safe wrappers `log` and `emitWarning`
 * exposed below; the wire-level i64-pointer marshalling stays in this
 * module.
 */

declare module "extism:host" {
  interface user {
    aggrega_log(level: I64, message_ptr: I64): void;
    aggrega_emit_warning(code_ptr: I64, message_ptr: I64, path_ptr: I64): void;
    aggrega_fetch_external_input(url_ptr: I64): I64;
  }
}

/**
 * Emit a log line to the Aggrega host. `level` is the severity
 * (0 = trace, 1 = debug, 2 = info, 3 = warn, 4 = error per the
 * project's log-level integer convention). `message` is the log
 * text.
 *
 * The `Host.getFunctions()` destructuring happens inside the wrapper
 * (rather than at module-init time) so the PDK can be imported in
 * non-Wasm runtimes (e.g., the round-trip test suite) without trying
 * to bind against a Wasm-only host. Per-call overhead is negligible
 * compared to the host call itself.
 */
export function log(level: number, message: string): void {
  const { aggrega_log } = Host.getFunctions();
  const mem = Memory.fromString(message);
  aggrega_log(BigInt(level), mem.offset);
}

/**
 * Emit a structured warning the host's build-time validator surfaces
 * via the warning channel. `code` is a short identifier the
 * validator categorises by; `message` is the warning text; `path` is
 * an optional JSON-Pointer-style location into the authored entity
 * (e.g., "components.payments-api.values.memory"). Pass an empty
 * string for `path` when the warning is not attached to a specific
 * authored location.
 */
export function emitWarning(code: string, message: string, path: string): void {
  const { aggrega_emit_warning } = Host.getFunctions();
  const codeMem = Memory.fromString(code);
  const msgMem = Memory.fromString(message);
  const pathMem = Memory.fromString(path);
  aggrega_emit_warning(codeMem.offset, msgMem.offset, pathMem.offset);
}

/**
 * Fetch the bytes of an external-input URL through the Aggrega host's
 * acquisition channel. The host serves the URL under its
 * external-input pin discipline: an already-pinned URL is served from
 * the content-addressed store; an unpinned URL is fetched and pinned
 * when acquisition is enabled, and refused with a clear message when
 * the closure is frozen.
 *
 * Failure is in-band: the host returns a JSON envelope carrying either
 * a base64 `content` field or an `error` field naming the URL and the
 * remedy; this wrapper throws an Error carrying that message. Plugin
 * code should treat a fetch error as a render failure to report, not a
 * condition to mask.
 */
export function fetchExternalInput(url: string): ArrayBuffer {
  const { aggrega_fetch_external_input } = Host.getFunctions();
  const urlMem = Memory.fromString(url);
  const offset = aggrega_fetch_external_input(urlMem.offset);
  if (offset === BigInt(0)) {
    throw new Error("aggrega_fetch_external_input: host returned no envelope");
  }
  const envelope: { content?: string; error?: string } = JSON.parse(
    Memory.find(offset).readString(),
  );
  if (envelope.error) {
    throw new Error(envelope.error);
  }
  if (envelope.content === undefined) {
    throw new Error(
      "aggrega_fetch_external_input: envelope carries neither content nor error",
    );
  }
  return Host.base64ToArrayBuffer(envelope.content);
}
