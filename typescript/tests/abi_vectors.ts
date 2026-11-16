// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

/**
 * Reads the shared payload vectors the aggrega-abi repository publishes in its
 * testdata directory. The vectors are not part of this repository: they are read
 * from an aggrega-abi checkout beside it (aggrega-pdk/ and aggrega-abi/ as
 * siblings).
 *
 * A clone of this repository with no aggrega-abi checkout beside it skips every
 * test that reads a vector, saying why: each such test passes `vectorsSkip` as
 * its skip option. Where the checkout is present, a run that could not read a
 * vector has verified nothing, so readVector refuses with a diagnostic naming the
 * path rather than skipping quietly.
 */

import { strict as assert } from "node:assert";
import { existsSync, readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));

// The aggrega-abi repository checked out beside this one.
const abiCheckout = join(__dirname, "..", "..", "..", "aggrega-abi");

/**
 * The skip reason for a test that reads a shared vector: a message when no
 * aggrega-abi checkout is beside this repository, undefined when one is.
 */
export const vectorsSkip: string | undefined = existsSync(abiCheckout)
  ? undefined
  : `aggrega-abi is not checked out beside aggrega-pdk (looked for ${abiCheckout}); its shared test vectors live there`;

// Resolves the directory holding the shared payload vectors. Resolution only;
// whether it can be read is decided in one place, by readVector.
function vectorDir(): string {
  return join(abiCheckout, "testdata");
}

/** Reads one shared vector file. */
export function readVector(name: string): string {
  const path = join(vectorDir(), name);
  try {
    return readFileSync(path, "utf8");
  } catch (err) {
    throw new Error(
      `shared payload vector ${name} is not readable at ${path}: ${String(err)}; the vectors are read from the testdata directory of the aggrega-abi repository, which must be checked out beside this one`,
    );
  }
}

/** Reads one shared canonical-CBOR vector and decodes its hex. */
export function readHexVector(name: string): Uint8Array {
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
