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

import {
  decodeCLIRunInput,
  decodeFetchExternalInputResponse,
  decodeParseSourceResponse,
  decodeSourceFileResponse,
  decodeSourcePackResponse,
  encodeParseInput,
  encodeSourceFileRequest,
  encodeSourcePackRequest,
  type SourceProjection,
} from "@aggrega-ai/aggrega-abi";

import { HostError } from "./host_error.ts";

declare module "extism:host" {
  interface user {
    aggrega_log(level: I64, message_ptr: I64): void;
    aggrega_emit_warning(code_ptr: I64, message_ptr: I64, path_ptr: I64): void;
    aggrega_fetch_external_input(url_ptr: I64): I64;
    aggrega_pack_source(request_ptr: I64): I64;
    aggrega_read_source_file(request_ptr: I64): I64;
    aggrega_parse_source(request_ptr: I64): I64;
    aggrega_cli_write_stdout(ptr: I64, len: I64): void;
    aggrega_cli_write_stderr(ptr: I64, len: I64): void;
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
  const envelope = decodeFetchExternalInputResponse(
    Memory.find(offset).readString(),
  );
  if (envelope.error) {
    throw new HostError(envelope.error_code ?? "", envelope.error);
  }
  if (envelope.content === undefined) {
    throw new Error(
      "aggrega_fetch_external_input: envelope carries neither content nor error",
    );
  }
  return Host.base64ToArrayBuffer(envelope.content);
}

/**
 * Ask the Aggrega host to pack a selection from the acquired source and
 * return the packed files. `op` names the shape of the selection ("file",
 * "dir", or "tree") and `path` names it relative to the source root (the
 * empty string names the root itself). The host resolves the selection under
 * the source root, refusing any path that escapes above it, and returns the
 * packed bytes as a source projection.
 *
 * This is the guest side of the host-listing source model: the host hands the
 * guest a name-only listing, the guest applies its own resolution logic, and
 * this call is the guest's request that the host read and pack the chosen
 * selection — the only point at which source bytes cross into the guest.
 *
 * Failure is in-band: the host returns a JSON envelope carrying either a
 * packed source projection or an error message; this wrapper throws an Error
 * carrying that message. Plugin code should treat a pack failure as a render
 * failure to report, not a condition to mask.
 */
export function packSource(op: string, path: string): SourceProjection {
  const { aggrega_pack_source } = Host.getFunctions();
  const reqMem = Memory.fromString(encodeSourcePackRequest({ op, path }));
  const offset = aggrega_pack_source(reqMem.offset);
  if (offset === BigInt(0)) {
    throw new Error("aggrega_pack_source: host returned no envelope");
  }
  const response = decodeSourcePackResponse(Memory.find(offset).readString());
  if (response.error) {
    throw new HostError(response.error_code ?? "", response.error);
  }
  if (response.source_projection === undefined) {
    throw new Error(
      "aggrega_pack_source: response carries neither a source projection nor an error",
    );
  }
  return response.source_projection;
}

/**
 * Ask the Aggrega host to read a single file from the acquired source and
 * return its bytes. `path` names the file relative to the source root. The
 * host resolves the path under the source root, refusing any path that escapes
 * above it, and returns the file's raw bytes.
 *
 * It is the content-based-selection primitive: a guest whose resolution logic
 * must inspect a file's content before choosing what to pack reads that file
 * through this call, then issues its `packSource` request.
 *
 * Failure is in-band, matching `packSource`: the host returns a JSON envelope
 * carrying either the file's base64 content or an error message; this wrapper
 * throws an Error carrying that message.
 */
export function readSourceFile(path: string): ArrayBuffer {
  const { aggrega_read_source_file } = Host.getFunctions();
  const reqMem = Memory.fromString(encodeSourceFileRequest({ path }));
  const offset = aggrega_read_source_file(reqMem.offset);
  if (offset === BigInt(0)) {
    throw new Error("aggrega_read_source_file: host returned no envelope");
  }
  const response = decodeSourceFileResponse(Memory.find(offset).readString());
  if (response.error) {
    throw new HostError(response.error_code ?? "", response.error);
  }
  return Host.base64ToArrayBuffer(response.content ?? "");
}

/**
 * Ask the Aggrega host to obtain a source's canonical structured tree by running
 * a parser for the given format through the host, and return the tree. `format`
 * is the syntactic format of the source (for example "json"); `source` is the raw
 * source bytes; `options` is an optional opaque options document the parser
 * interprets. The host resolves the parser for the format, runs it, and returns
 * the tree it produced (a parsed JSON value).
 *
 * This is the guest side of host-mediated producer consumption: a consumer plugin
 * obtains a producer's output through the host rather than invoking a producer
 * guest itself. The host bounds it to a single level — a producer reached this way
 * that calls `parseSource` again is denied — so a consumer calls it once per
 * source and may consume many sources in sequence.
 *
 * Failure is in-band: the host returns a JSON envelope carrying either the tree or
 * an error message naming the failure; this wrapper throws an Error carrying that
 * message. Plugin code should treat a parse-source error as a failure to report,
 * not a condition to mask.
 */
export function parseSource(
  format: string,
  source: ArrayBufferLike,
  options?: unknown,
): unknown {
  const { aggrega_parse_source } = Host.getFunctions();
  const reqMem = Memory.fromString(
    encodeParseInput({
      format,
      bytes: Host.arrayBufferToBase64(source as ArrayBuffer),
      options,
    }),
  );
  const offset = aggrega_parse_source(reqMem.offset);
  if (offset === BigInt(0)) {
    throw new Error("aggrega_parse_source: host returned no envelope");
  }
  const response = decodeParseSourceResponse(Memory.find(offset).readString());
  if (response.error) {
    throw new HostError(response.error_code ?? "", response.error);
  }
  if (response.tree === undefined) {
    throw new Error(
      "aggrega_parse_source: response carries neither a tree nor an error",
    );
  }
  return response.tree;
}

/**
 * Stream a chunk of bytes to the Aggrega host's standard-output stream for
 * the running CLI command. The host copies the bytes out of guest memory and
 * writes them, incrementally, to the terminal's stdout — there is no single
 * output payload, so a plugin calls `writeStdout` as many times as it needs
 * while it runs. This binding is granted only when the guest runs as a CLI
 * plugin; a build-time load never binds it.
 */
export function writeStdout(data: ArrayBufferLike): void {
  const { aggrega_cli_write_stdout } = Host.getFunctions();
  const mem = Memory.fromBuffer(data);
  aggrega_cli_write_stdout(mem.offset, mem.len);
}

/**
 * Stream a chunk of bytes to the Aggrega host's standard-error stream for the
 * running CLI command. It is the stderr sibling of `writeStdout` and carries
 * the same runtime-only grant.
 */
export function writeStderr(data: ArrayBufferLike): void {
  const { aggrega_cli_write_stderr } = Host.getFunctions();
  const mem = Memory.fromBuffer(data);
  aggrega_cli_write_stderr(mem.offset, mem.len);
}

/**
 * Read the `cli_run` input payload the host hands a CLI plugin on each
 * invocation and return the argument vector — the tokens that follow the
 * resolved subcommand name on the command line, in invocation order. The
 * plugin parses these tokens as its own flags and positional arguments,
 * exactly as a standalone command-line program reads the arguments after its
 * name.
 */
export function cliArgs(): string[] {
  return decodeCLIRunInput(Host.inputString()).args;
}
