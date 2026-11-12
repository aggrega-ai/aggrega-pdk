// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

/**
 * Wire tests for the `aggrega_parse_source` call, driven against the shared
 * payload vectors the ABI package publishes rather than against bytes produced
 * here. That is what makes them discriminating: a kit that built its request with
 * the wrong encoder, or read its reply with the wrong decoder, produces bytes that
 * do not match a vector an independent instrument has already anchored — whereas a
 * kit checked only against its own output round-trips cleanly however wrong both
 * halves are, because the same mistake is on both sides.
 *
 * The tests exercise the two wire functions and never the host call itself: the
 * host bindings exist only inside the guest runtime, so a test that reached the
 * call would have nothing to call.
 */

import { test } from "node:test";
import { strict as assert } from "node:assert";
import { readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

import {
  encodeParseSourceRequest,
  decodeParseSourceReply,
} from "../src/host_funcs.ts";
import { HostError } from "../src/host_error.ts";

const __dirname = dirname(fileURLToPath(import.meta.url));

// Resolves the directory holding the shared payload vectors. Resolution only;
// whether it can be read is decided in one place, by readVector.
function vectorDir(): string {
  return join(__dirname, "..", "..", "..", "aggrega-abi", "testdata");
}

// Reads one shared vector file. A run that could not read the vectors has
// verified nothing, so this refuses with a diagnostic naming the path rather than
// skipping quietly.
function readVector(name: string): string {
  const path = join(vectorDir(), name);
  try {
    return readFileSync(path, "utf8");
  } catch (err) {
    throw new Error(
      `shared payload vector ${name} is not readable at ${path}: ${String(err)}; the vectors are read from the testdata directory of the aggrega-abi repository, which must be checked out beside this one`,
    );
  }
}

// Reads one shared canonical-CBOR vector and decodes its hex.
function readHexVector(name: string): Uint8Array {
  const raw = readVector(name).trim();
  assert.equal(
    raw.length % 2,
    0,
    `shared vector ${name} does not hold an even number of hex digits`,
  );
  const out = new Uint8Array(raw.length / 2);
  for (let i = 0; i < out.length; i++) {
    const byte = Number.parseInt(raw.slice(i * 2, i * 2 + 2), 16);
    assert.ok(
      Number.isInteger(byte),
      `shared vector ${name} does not hold decodable hex`,
    );
    out[i] = byte;
  }
  return out;
}

function hex(b: Uint8Array): string {
  return Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
}

// Copies a Uint8Array into an exactly-sized ArrayBuffer, which is the shape the
// reply decoder reads.
function toBuffer(b: Uint8Array): ArrayBuffer {
  const out = new ArrayBuffer(b.byteLength);
  new Uint8Array(out).set(b);
  return out;
}

// The request the kit builds is byte-identical to the shared ParseInput vector,
// for the very field values that vector's companion document declares. A kit
// still building a text request fails here, because a text document and the
// canonical binary encoding of the same value are different bytes.
test("the parse-source request matches the shared ParseInput vector", () => {
  // The companion document carries the field values in the textual encoding,
  // where the source bytes are base64 — which is the shape the payload type's
  // own field holds. Read them from there rather than restating them, so the two
  // vectors cannot drift apart from this test.
  const golden = JSON.parse(readVector("ParseInput.golden.json")) as {
    format: string;
    bytes: string;
    options?: unknown;
  };

  const got = new Uint8Array(
    encodeParseSourceRequest({
      format: golden.format,
      bytes: golden.bytes,
      options: golden.options,
    }),
  );
  const want = readHexVector("ParseInput.cbor.hex");
  assert.equal(
    hex(got),
    hex(want),
    "request bytes do not match the shared ParseInput vector",
  );
});

// The kit reads a reported failure out of the shared ParseSourceResponse vector
// and surfaces it as a HostError carrying both the machine-matchable code and the
// message.
test("a reported parse-source failure surfaces as a HostError from the shared vector", () => {
  const golden = JSON.parse(readVector("ParseSourceResponse.golden.json")) as {
    error: string;
    hostfunc_error_code: string;
  };

  assert.throws(
    () =>
      decodeParseSourceReply(
        toBuffer(readHexVector("ParseSourceResponse.cbor.hex")),
      ),
    (err: unknown) => {
      assert.ok(
        err instanceof HostError,
        `the failure surfaces as a HostError, got ${String(err)}`,
      );
      assert.equal(err.code, golden.hostfunc_error_code);
      assert.equal(err.message, golden.error);
      return true;
    },
  );
});

// The kit reads the success shape too — the one production guests take, and the
// only one carrying the tree.
//
// Both shapes are asserted because each hides the other's defect: a decoder that
// only reads the failure fields passes any test built from failure replies alone,
// and one that only reads the tree passes any test built from success replies
// alone.
//
// The vector is authored from the canonical encoding's own rules rather than
// captured, and it is short enough to read off:
//
//   a1                      map, 1 pair
//   64 74 72 65 65          text, 4 bytes: "tree"
//   47 7b 22 61 22 3a 31 7d byte run, 7 bytes: {"a":1}
//
// One pair is what a tree-only envelope carries, since the two failure fields are
// omitted when empty; and the tree travels as a byte run holding its own document
// verbatim, which is why an over- or under-declared pair count would move the head
// byte and fail here rather than pass silently.
test("the parse-source success shape decodes to its tree", () => {
  const reply = new Uint8Array([
    0xa1, 0x64, 0x74, 0x72, 0x65, 0x65, 0x47, 0x7b, 0x22, 0x61, 0x22, 0x3a,
    0x31, 0x7d,
  ]);
  assert.deepEqual(decodeParseSourceReply(toBuffer(reply)), { a: 1 });
});

// The reply decoder is not silently tolerant of a textual reply. Without this, a
// host left on the other encoding would be reported as some downstream shape error
// rather than as an undecodable reply, and the two are not the same diagnosis.
test("a textual parse-source reply is refused", () => {
  const textual = new TextEncoder().encode(`{"tree":{"a":1}}`);
  assert.throws(
    () => decodeParseSourceReply(toBuffer(textual)),
    "a textual reply decoded without error; the reply slot carries the binary encoding only",
  );
});

// An envelope carrying neither a tree nor a failure is reported rather than
// passing as an empty tree. The distinction matters because an empty tree is
// itself a legitimate outcome, so a lenient decode would make "the host said
// nothing" and "the parser produced nothing" indistinguishable.
test("an empty parse-source envelope is an error rather than an empty tree", () => {
  // a0: map, 0 pairs — every field omitted.
  assert.throws(() => decodeParseSourceReply(toBuffer(new Uint8Array([0xa0]))));
});
