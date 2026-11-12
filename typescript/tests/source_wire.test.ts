// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

/**
 * Wire tests for the `aggrega_pack_source` and `aggrega_read_source_file` calls,
 * driven against the shared payload vectors the ABI package publishes rather than
 * against bytes produced here. That is what makes them discriminating: a kit that
 * built its request with the wrong encoder, or read its reply with the wrong
 * decoder, produces bytes that do not match a vector an independent instrument has
 * already anchored — whereas a kit checked only against its own output round-trips
 * cleanly however wrong both halves are, because the same mistake is on both
 * sides.
 *
 * The tests exercise the wire functions and never the host calls themselves: the
 * host bindings exist only inside the guest runtime, so a test that reached one
 * would have nothing to call.
 */

import { test } from "node:test";
import { strict as assert } from "node:assert";
import { readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

import {
  encodePackSourceRequest,
  decodePackSourceReply,
  encodeReadSourceFileRequest,
  decodeReadSourceFileReply,
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
// reply decoders read.
function toBuffer(b: Uint8Array): ArrayBuffer {
  const out = new ArrayBuffer(b.byteLength);
  new Uint8Array(out).set(b);
  return out;
}

// The pack-source request the kit builds is byte-identical to the shared
// SourcePackRequest vector, for the very field values that vector's companion
// document declares. A kit still building a text request fails here, because a
// text document and the canonical binary encoding of the same value are different
// bytes.
test("the pack-source request matches the shared SourcePackRequest vector", () => {
  const golden = JSON.parse(readVector("SourcePackRequest.golden.json")) as {
    op: string;
    path: string;
  };

  const got = new Uint8Array(
    encodePackSourceRequest({ op: golden.op, path: golden.path }),
  );
  const want = readHexVector("SourcePackRequest.cbor.hex");
  assert.equal(
    hex(got),
    hex(want),
    "request bytes do not match the shared SourcePackRequest vector",
  );
});

// The content-read sibling of the pack-source request assertion.
test("the read-source-file request matches the shared SourceFileRequest vector", () => {
  const golden = JSON.parse(readVector("SourceFileRequest.golden.json")) as {
    path: string;
  };

  const got = new Uint8Array(
    encodeReadSourceFileRequest({ path: golden.path }),
  );
  const want = readHexVector("SourceFileRequest.cbor.hex");
  assert.equal(
    hex(got),
    hex(want),
    "request bytes do not match the shared SourceFileRequest vector",
  );
});

// The kit reads the pack-source success shape out of the shared vector — the
// shape every production provider guest takes, and the only one carrying the
// packed files.
//
// This vector carries the projection with both failure fields omitted; the
// failure test below carries the failure fields with the projection omitted. Both
// directions are asserted because each hides the other's defect: a decoder that
// only reads the failure fields passes any test built from failure replies alone,
// and one that only reads the projection passes any test built from success
// replies alone.
test("the pack-source success shape decodes to its packed files", () => {
  // The companion document carries each file's content base64, which is the
  // shape the payload type's own field holds in this package.
  const golden = JSON.parse(readVector("SourcePackResponse.golden.json")) as {
    source_projection: { files: { path: string; content: string }[] };
  };

  const got = decodePackSourceReply(
    toBuffer(readHexVector("SourcePackResponse.cbor.hex")),
  );
  assert.deepEqual(got, golden.source_projection);
});

// The kit reads a reported pack-source failure out of the shared error-envelope
// vector and surfaces it as a HostError carrying both the machine-matchable code
// and the message.
test("a reported pack-source failure surfaces as a HostError from the shared vector", () => {
  const golden = JSON.parse(
    readVector("ErrorEnvelopeSourcePackResponse.golden.json"),
  ) as { error: string; hostfunc_error_code: string };

  assert.throws(
    () =>
      decodePackSourceReply(
        toBuffer(readHexVector("ErrorEnvelopeSourcePackResponse.cbor.hex")),
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

// The kit reads the file's content out of the shared SourceFileResponse vector,
// in the payload field's own shape.
test("the read-source-file success shape decodes to its content", () => {
  const golden = JSON.parse(readVector("SourceFileResponse.golden.json")) as {
    content: string;
  };

  const got = decodeReadSourceFileReply(
    toBuffer(readHexVector("SourceFileResponse.cbor.hex")),
  );
  assert.equal(got, golden.content);
});

// The content-read failure shape surfaces as a HostError — the other direction of
// the omission property from the success vector above.
test("a reported read-source-file failure surfaces as a HostError from the shared vector", () => {
  const golden = JSON.parse(
    readVector("ErrorEnvelopeSourceFileResponse.golden.json"),
  ) as { error: string; hostfunc_error_code: string };

  assert.throws(
    () =>
      decodeReadSourceFileReply(
        toBuffer(readHexVector("ErrorEnvelopeSourceFileResponse.cbor.hex")),
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

// Neither source reply decoder is silently tolerant of a textual reply. Without
// this, a host left on the other encoding would be reported as some downstream
// shape error rather than as an undecodable reply, and the two are not the same
// diagnosis.
test("a textual source reply is refused by both decoders", () => {
  const textualPack = new TextEncoder().encode(
    `{"source_projection":{"files":[]}}`,
  );
  assert.throws(
    () => decodePackSourceReply(toBuffer(textualPack)),
    "a textual pack-source reply decoded without error; the reply slot carries the binary encoding only",
  );
  const textualFile = new TextEncoder().encode(`{"content":"aGk="}`);
  assert.throws(
    () => decodeReadSourceFileReply(toBuffer(textualFile)),
    "a textual read-source-file reply decoded without error; the reply slot carries the binary encoding only",
  );
});

// A pack-source envelope carrying neither a projection nor a failure is reported
// rather than passing as an empty pack, so "the host said nothing" and "the
// selection matched nothing" stay distinguishable.
test("an empty pack-source envelope is an error rather than an empty pack", () => {
  // a0: map, 0 pairs — every field omitted.
  assert.throws(() => decodePackSourceReply(toBuffer(new Uint8Array([0xa0]))));
});

// The deliberate asymmetry with the pack-source decoder: an empty content field
// is a legitimate read of an empty file, and the field is omitted when empty, so
// a reply with no failure and no content yields the empty string rather than an
// error.
//
// Worth pinning rather than inferring: making this an error would refuse every
// empty file in an acquired source, and making the pack-source case lenient would
// let a silent host produce an empty pack.
test("an empty read-source-file envelope is the empty file", () => {
  // a0: map, 0 pairs — every field omitted.
  assert.equal(
    decodeReadSourceFileReply(toBuffer(new Uint8Array([0xa0]))),
    "",
  );
});

// The map head byte of each shared source vector, read from the canonical
// encoding's own rules rather than from the encoder that wrote them. A head
// disagreeing with its body round-trips cleanly through the generator's own two
// halves, so reading the head is what a same-generator comparison cannot do.
//
// The heads assert the omission property in both directions at once: a success
// envelope carries only its payload key, and a failure envelope carries only the
// two failure keys.
test("each shared source vector declares the pair count its body carries", () => {
  const cases: [string, number, string][] = [
    ["SourcePackRequest.cbor.hex", 0xa2, "two required fields: op and path"],
    ["SourceFileRequest.cbor.hex", 0xa1, "one required field: path"],
    [
      "SourcePackResponse.cbor.hex",
      0xa1,
      "the projection alone; both failure fields omitted",
    ],
    [
      "SourceFileResponse.cbor.hex",
      0xa1,
      "the content alone; both failure fields omitted",
    ],
    [
      "ErrorEnvelopeSourcePackResponse.cbor.hex",
      0xa2,
      "message and code; the projection omitted",
    ],
    [
      "ErrorEnvelopeSourceFileResponse.cbor.hex",
      0xa2,
      "message and code; the content omitted",
    ],
  ];
  for (const [vector, want, why] of cases) {
    const b = readHexVector(vector);
    assert.ok(b.length > 0, `${vector} is empty`);
    assert.equal(b[0], want, `${vector} head byte (${why})`);
  }
});
