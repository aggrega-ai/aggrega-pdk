// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

/**
 * Wire tests for the `aggrega_fetch_external_input` reply, driven against the
 * shared payload vectors the ABI package publishes rather than against bytes
 * produced here, for the reason source_wire.test.ts gives: a reader checked only
 * against its own encoder round-trips cleanly however wrong both halves are.
 *
 * The tests exercise the reply reader and never the host call itself: the host
 * bindings exist only inside the guest runtime, so a test that reached one would
 * have nothing to call.
 */

import { test } from "node:test";
import { strict as assert } from "node:assert";
import { readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

import {
  CodeFetchFailed,
  encodeCBORFetchExternalInputResponse,
} from "@aggrega-ai/aggrega-abi";

import { decodeFetchExternalInputReply } from "../src/host_funcs.ts";
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
// reply reader takes.
function toBuffer(b: Uint8Array): ArrayBuffer {
  const out = new ArrayBuffer(b.byteLength);
  new Uint8Array(out).set(b);
  return out;
}

// The kit reads the fetched content out of the shared success vector, in the
// payload field's own shape: base64, which the companion document carries too.
test("the fetch success shape decodes to its content", () => {
  const golden = JSON.parse(
    readVector("FetchExternalInputResponseContent.golden.json"),
  ) as { content: string };

  const got = decodeFetchExternalInputReply(
    toBuffer(readHexVector("FetchExternalInputResponseContent.cbor.hex")),
  );
  assert.equal(got, golden.content);
});

// A zero-length payload is a success yielding the empty content. Its reply
// carries no field at all, because the content field is omitted when empty, so
// a reader that took an absent content key for a failure fails here.
test("a zero-length fetched payload is the empty content, not a failure", () => {
  assert.equal(
    decodeFetchExternalInputReply(
      toBuffer(readHexVector("FetchExternalInputResponseEmptyContent.cbor.hex")),
    ),
    "",
  );
});

// The kit reads a reported failure out of the shared failure vector and surfaces
// it as a HostError carrying both the machine-matchable code and the message.
test("a reported fetch failure surfaces as a HostError from the shared vector", () => {
  const golden = JSON.parse(
    readVector("FetchExternalInputResponse.golden.json"),
  ) as { error: string; hostfunc_error_code: string };

  assert.throws(
    () =>
      decodeFetchExternalInputReply(
        toBuffer(readHexVector("FetchExternalInputResponse.cbor.hex")),
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

// A reply carrying a failure code and no message is still a failure. Without
// this, such a reply would carry no content and no message, which is exactly the
// shape of the zero-length payload, and would read as a successful empty fetch.
test("a fetch reply carrying a code alone is a failure", () => {
  const reply = encodeCBORFetchExternalInputResponse({
    hostfunc_error_code: CodeFetchFailed,
  });
  assert.throws(
    () => decodeFetchExternalInputReply(toBuffer(reply)),
    (err: unknown) => {
      assert.ok(
        err instanceof HostError,
        `the failure surfaces as a HostError, got ${String(err)}`,
      );
      assert.equal(err.code, CodeFetchFailed);
      assert.ok(err.message.length > 0, "the failure carries a message");
      return true;
    },
  );
});

// The reply reader is not silently tolerant of a textual reply. Without this, a
// host left on the textual encoding would surface as some downstream shape error
// rather than as an undecodable reply, and the two are not the same diagnosis.
test("a textual fetch reply is refused", () => {
  const textual = new TextEncoder().encode(`{"content":"aGVsbG8="}`);
  assert.throws(
    () => decodeFetchExternalInputReply(toBuffer(textual)),
    "a textual fetch reply decoded without error; the reply slot carries the binary encoding only",
  );
});

// The map head byte of each shared fetch vector, read from the canonical
// encoding's own rules, and the content vector's value head: a byte string
// (major type 2), which a text-string or base64 carry would not produce.
test("each shared fetch vector declares the pair count its body carries", () => {
  const cases: [string, number, string][] = [
    [
      "FetchExternalInputResponseContent.cbor.hex",
      0xa1,
      "the content alone; both failure fields omitted",
    ],
    [
      "FetchExternalInputResponseEmptyContent.cbor.hex",
      0xa0,
      "no field at all: an empty content is omitted",
    ],
    [
      "FetchExternalInputResponse.cbor.hex",
      0xa2,
      "message and code; the content omitted",
    ],
  ];
  for (const [vector, want, why] of cases) {
    const b = readHexVector(vector);
    assert.ok(b.length > 0, `${vector} is empty`);
    assert.equal(b[0], want, `${vector} head byte (${why})`);
  }
  // a1, 67 "content", then the value head: 0x40 | length marks a byte string.
  const b = readHexVector("FetchExternalInputResponseContent.cbor.hex");
  assert.ok(
    b.length >= 10 && (b[9] & 0xe0) === 0x40,
    `content value head in ${hex(b)}; want a byte string (major type 2)`,
  );
});
