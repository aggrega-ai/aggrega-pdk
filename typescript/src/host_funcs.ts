// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

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
  decodeCBORFetchExternalInputResponse,
  decodeCBORParseSourceResponse,
  decodeCBORSourceFileResponse,
  decodeCBORSourcePackResponse,
  decodeCLIRunInput,
  encodeCBORParseInput,
  encodeCBORSourceFileRequest,
  encodeCBORSourcePackRequest,
  type ParseInput,
  type SourceFileRequest,
  type SourcePackRequest,
  type SourceProjection,
} from "@aggrega-ai/aggrega-abi";

import { HostError } from "./host_error.ts";

declare module "extism:host" {
  interface user {
    aggrega_log(level: I64, message_ptr: I64): void;
    aggrega_emit_warning(code_ptr: I64, message_ptr: I64, path_ptr: I64): void;
    // The URL is a bare string rather than a payload document, so it is
    // allocated with `Memory.fromString`; the reply is a payload document in
    // canonical CBOR and is read with `readBytes`, for the reason given below.
    aggrega_fetch_external_input(url_ptr: I64): I64;
    // Both slots on each of the three payload-carrying callbacks below carry
    // canonical CBOR. The declaration is a pointer either way, so the shape is
    // carried by how the memory behind each pointer is allocated and read —
    // `Memory.fromBuffer` and `readBytes` rather than `Memory.fromString` and
    // `readString` — and not by these signatures.
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
 * The URL travels as UTF-8 text, because it is a bare string rather than a
 * payload document. The reply carries canonical CBOR, success and failure
 * alike, with the fetched bytes as a raw byte string: a payload that is not
 * valid UTF-8 arrives intact, and a zero-length payload returns an empty
 * buffer rather than an error. Plugin code calling this wrapper needs no CBOR
 * library and hand-parses nothing.
 *
 * Failure is in-band: the host returns an error envelope rather than trapping,
 * and this wrapper throws it as a `HostError` whose message names the URL and
 * the remedy and whose code is one of the `CodeFetch*` constants. Plugin code
 * should treat a fetch error as a render failure to report, not a condition to
 * mask.
 */
export function fetchExternalInput(url: string): ArrayBuffer {
  const { aggrega_fetch_external_input } = Host.getFunctions();
  const urlMem = Memory.fromString(url);
  const offset = aggrega_fetch_external_input(urlMem.offset);
  if (offset === BigInt(0)) {
    throw new Error("aggrega_fetch_external_input: host returned no envelope");
  }
  return Host.base64ToArrayBuffer(
    decodeFetchExternalInputReply(Memory.find(offset).readBytes()),
  );
}

/**
 * Reads one `aggrega_fetch_external_input` reply and returns the fetched
 * content as the payload field holds it — base64, matching the textual
 * encoding's own field type — throwing the failure it reports.
 *
 * The reply slot carries canonical CBOR in both the success and the failure
 * case, so one decode entry point reads both. A reply carrying an error message
 * or an error code is a failure and is thrown as a `HostError`. Any other reply
 * is a success, and its content is the payload: the field is omitted when
 * empty, so a zero-length payload arrives as a reply with no field at all and
 * yields the empty string. Reading the failure from the failure fields, rather
 * than from the content key's absence, is what keeps that empty payload
 * distinguishable from a failure.
 *
 * It returns the field rather than the decoded buffer for the reason
 * `decodeReadSourceFileReply` gives: base64-decoding is a guest-runtime
 * capability, and keeping that call in `fetchExternalInput` leaves this
 * function reachable off Wasm.
 */
export function decodeFetchExternalInputReply(reply: ArrayBuffer): string {
  const response = decodeCBORFetchExternalInputResponse(new Uint8Array(reply));
  if (response.error || response.hostfunc_error_code) {
    const code = response.hostfunc_error_code ?? "";
    throw new HostError(
      code,
      response.error ||
        `aggrega_fetch_external_input: the host reported failure ${code}`,
    );
  }
  return response.content ?? "";
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
 * Both slots on this call carry canonical CBOR, so the request is allocated as an
 * opaque byte run rather than as UTF-8 text and the reply is read the same way.
 * Plugin code calling this wrapper needs no CBOR library and hand-parses nothing.
 *
 * Failure is in-band: the host returns an envelope carrying either a packed
 * source projection or an error message; this wrapper throws an Error carrying
 * that message. Plugin code should treat a pack failure as a render failure to
 * report, not a condition to mask.
 */
export function packSource(op: string, path: string): SourceProjection {
  const { aggrega_pack_source } = Host.getFunctions();
  const reqMem = Memory.fromBuffer(encodePackSourceRequest({ op, path }));
  const offset = aggrega_pack_source(reqMem.offset);
  if (offset === BigInt(0)) {
    throw new Error("aggrega_pack_source: host returned no envelope");
  }
  return decodePackSourceReply(Memory.find(offset).readBytes());
}

/**
 * Builds the request bytes for one `aggrega_pack_source` call.
 *
 * The request slot carries canonical CBOR, so the bytes are an opaque byte run
 * rather than UTF-8 text, and they come from the payload type's own generated
 * encoder rather than from anything assembled here.
 *
 * Split out from `packSource` so the wire is exercisable where the call is not:
 * the host bindings exist only inside the guest runtime, so a test can reach this
 * and cannot reach the call.
 *
 * Deliberately not re-exported from the package entry point — a plugin author
 * calls `packSource`.
 */
export function encodePackSourceRequest(
  request: SourcePackRequest,
): ArrayBuffer {
  return toExactBuffer(encodeCBORSourcePackRequest(request));
}

/**
 * Reads one `aggrega_pack_source` reply and returns the packed source it carries,
 * throwing the failure it reports.
 *
 * The reply slot carries canonical CBOR in both the success and the failure case:
 * a guest reads one slot and cannot know before decoding which of the two it
 * holds, so one decode entry point reads both. A reported failure is thrown as a
 * `HostError` carrying the host's machine-matchable code beside its message; a
 * reply carrying neither a projection nor a failure is itself an error, so an
 * empty reply cannot pass as an empty pack.
 */
export function decodePackSourceReply(reply: ArrayBuffer): SourceProjection {
  const response = decodeCBORSourcePackResponse(new Uint8Array(reply));
  if (response.error) {
    throw new HostError(response.hostfunc_error_code ?? "", response.error);
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
 * Both slots carry canonical CBOR, on the same terms as `packSource`.
 *
 * Failure is in-band, matching `packSource`: the host returns an envelope
 * carrying either the file's content or an error message; this wrapper throws an
 * Error carrying that message.
 */
export function readSourceFile(path: string): ArrayBuffer {
  const { aggrega_read_source_file } = Host.getFunctions();
  const reqMem = Memory.fromBuffer(encodeReadSourceFileRequest({ path }));
  const offset = aggrega_read_source_file(reqMem.offset);
  if (offset === BigInt(0)) {
    throw new Error("aggrega_read_source_file: host returned no envelope");
  }
  return Host.base64ToArrayBuffer(
    decodeReadSourceFileReply(Memory.find(offset).readBytes()),
  );
}

/**
 * Builds the request bytes for one `aggrega_read_source_file` call.
 *
 * The request slot carries canonical CBOR, on the same terms as
 * `encodePackSourceRequest`, and it is split out from `readSourceFile` for the
 * same reason.
 *
 * Deliberately not re-exported from the package entry point — a plugin author
 * calls `readSourceFile`.
 */
export function encodeReadSourceFileRequest(
  request: SourceFileRequest,
): ArrayBuffer {
  return toExactBuffer(encodeCBORSourceFileRequest(request));
}

/**
 * Reads one `aggrega_read_source_file` reply and returns the file's content as
 * the payload field holds it — base64, matching the textual encoding's own field
 * type — throwing the failure it reports.
 *
 * It returns the field rather than the decoded buffer because base64-decoding is
 * a guest-runtime capability: keeping that call in `readSourceFile` is what
 * leaves this function reachable off Wasm, which is the whole point of splitting
 * it out. The generated decoder re-encodes the raw byte run to base64 on the way
 * in, so the field this returns is the file's bytes in the payload's own shape.
 *
 * Unlike `decodePackSourceReply` this does not treat an absent payload as an
 * error: an empty file is a legitimate read, and its reply carries no content key
 * at all because the field is omitted when empty. So a reply with no failure
 * yields the empty string, which is the empty file.
 */
export function decodeReadSourceFileReply(reply: ArrayBuffer): string {
  const response = decodeCBORSourceFileResponse(new Uint8Array(reply));
  if (response.error) {
    throw new HostError(response.hostfunc_error_code ?? "", response.error);
  }
  return response.content ?? "";
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
 * Failure is in-band: the host returns an envelope carrying either the tree or an
 * error message naming the failure; this wrapper throws an Error carrying that
 * message. Plugin code should treat a parse-source error as a failure to report,
 * not a condition to mask.
 */
export function parseSource(
  format: string,
  source: ArrayBufferLike,
  options?: unknown,
): unknown {
  const { aggrega_parse_source } = Host.getFunctions();
  const reqMem = Memory.fromBuffer(
    encodeParseSourceRequest({
      format,
      bytes: Host.arrayBufferToBase64(source as ArrayBuffer),
      options,
    }),
  );
  const offset = aggrega_parse_source(reqMem.offset);
  if (offset === BigInt(0)) {
    throw new Error("aggrega_parse_source: host returned no envelope");
  }
  return decodeParseSourceReply(Memory.find(offset).readBytes());
}

/**
 * Builds the request bytes for one `aggrega_parse_source` call.
 *
 * The request slot carries canonical CBOR, so the bytes are an opaque byte run
 * rather than UTF-8 text, and they come from the payload type's own generated
 * encoder rather than from anything assembled here. The payload's `bytes` field
 * holds base64, and the generated encoder decodes that back to the raw run before
 * writing it, so the source bytes reach the host once rather than twice-encoded.
 *
 * It takes the payload value rather than the raw source because base64-encoding a
 * buffer is a guest-runtime capability: keeping that call in `parseSource` is what
 * leaves this function reachable off Wasm, which is the whole point of splitting
 * it out. The host bindings exist only inside the guest runtime, so a test can
 * reach this and cannot reach the call.
 *
 * Deliberately not re-exported from the package entry point — a plugin author
 * calls `parseSource`.
 */
export function encodeParseSourceRequest(request: ParseInput): ArrayBuffer {
  return toExactBuffer(encodeCBORParseInput(request));
}

/**
 * Copies an encoder's output into an exactly-sized buffer rather than handing
 * over the view's own: a Uint8Array may be a window onto a larger buffer, and
 * passing that buffer would hand the host bytes outside the payload.
 *
 * One copy of this shared by every request builder on this wire, so the three
 * cannot drift on the one thing that decides how many bytes the host reads.
 */
function toExactBuffer(encoded: Uint8Array): ArrayBuffer {
  const out = new ArrayBuffer(encoded.byteLength);
  new Uint8Array(out).set(encoded);
  return out;
}

/**
 * Reads one `aggrega_parse_source` reply and returns the canonical tree it
 * carries, throwing the failure it reports.
 *
 * The reply slot carries canonical CBOR in both the success and the failure case:
 * a guest reads one slot and cannot know before decoding which of the two it
 * holds, so one decoder reads both. A reported failure is thrown as a `HostError`
 * carrying the host's machine-matchable code beside its message; a reply carrying
 * neither a tree nor a failure is itself an error, so an empty reply cannot pass
 * as an empty tree.
 */
export function decodeParseSourceReply(reply: ArrayBuffer): unknown {
  const response = decodeCBORParseSourceResponse(new Uint8Array(reply));
  if (response.error) {
    throw new HostError(response.hostfunc_error_code ?? "", response.error);
  }
  if (response.tree === undefined) {
    throw new Error(
      "aggrega_parse_source: reply carries neither a tree nor an error",
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
