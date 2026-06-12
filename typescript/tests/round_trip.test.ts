// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Round-trip mirroring tests for the Aggrega PDK socket types.
 *
 * Each test:
 *  1. Constructs a fixture value of the PDK type.
 *  2. Serialises via JSON.stringify.
 *  3. Validates against the corresponding JSON Schema in the sibling
 *     `aggrega-schemas` repository using Ajv 2020-12.
 *  4. Deserialises back via JSON.parse and asserts deep equality.
 *
 * Schemas live alongside the umbrella checkout at
 * `../../aggrega-schemas/json/` relative to this crate root.
 * Tests fail when the schema is unreachable rather than silently
 * passing; disconnected dev opts out via `node --test --test-skip-pattern=round_trip`.
 */

import { test } from "node:test";
import { strict as assert } from "node:assert";
import { readFileSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import Ajv from "ajv/dist/2020.js";

import {
  API_VERSION,
  type MutatorInput,
  type MutatorOutput,
  type PolicyValidatorInput,
  type PolicyValidatorOutput,
} from "../src/index.ts";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

function loadSchema(name: string): object {
  const schemaPath = join(
    __dirname,
    "..",
    "..",
    "..",
    "aggrega-schemas",
    "json",
    name,
  );
  if (!existsSync(schemaPath)) {
    throw new Error(
      `schema ${name} not reachable at ${schemaPath}: this test requires ` +
        `the umbrella checkout layout (aggrega-pdk/ and aggrega-schemas/ ` +
        `as siblings). Skip with --test-skip-pattern=round_trip.`,
    );
  }
  return JSON.parse(readFileSync(schemaPath, "utf-8"));
}

function makeValidator(schemaName: string) {
  const ajv = new Ajv({ strict: false });
  const schema = loadSchema(schemaName);
  return ajv.compile(schema);
}

test("API_VERSION is the v1alpha1 contract floor", () => {
  assert.equal(API_VERSION, "v1alpha1");
});

test("MutatorInput round-trips through wasm-mutator schema", () => {
  const validate = makeValidator("wasm-mutator-v1alpha1.schema.json");

  const fixture: MutatorInput = {
    compilation_context: {
      release_destination: "aws-prod/eks-main",
      release_name: "payments",
      environment: "prod",
    },
    graph: {
      nodes: {
        "payments-api": {
          component: "core/web-service",
          resolved_version: "1.4.2",
          labels: { team: "payments" },
        },
      },
    },
  };

  const serialised = JSON.parse(JSON.stringify(fixture));
  const ok = validate(serialised);
  assert.equal(ok, true, `schema validation failed: ${JSON.stringify(validate.errors)}`);

  const round = JSON.parse(JSON.stringify(fixture)) as MutatorInput;
  assert.deepStrictEqual(round, fixture);
});

test("MutatorOutput round-trips through wasm-mutator schema", () => {
  const validate = makeValidator("wasm-mutator-v1alpha1.schema.json");

  const fixture: MutatorOutput = {
    patches: [
      [
        {
          op: "add",
          path: "/nodes/payments-api/labels/compliance",
          value: "soc2",
        },
      ],
    ],
  };

  const serialised = JSON.parse(JSON.stringify(fixture));
  const ok = validate(serialised);
  assert.equal(ok, true, `schema validation failed: ${JSON.stringify(validate.errors)}`);
});

test("PolicyValidatorInput round-trips through wasm-policy-validator schema", () => {
  const validate = makeValidator("wasm-policy-validator-v1alpha1.schema.json");

  const fixture: PolicyValidatorInput = {
    graph: {
      nodes: {
        "payments-api": {
          component: "core/web-service",
          resolved_version: "1.4.2",
          labels: { team: "payments" },
          features: {
            observability: {
              type: "feature/observability",
              resolved_version: "2.0.1",
              values: { tracing_enabled: true },
              origin: "explicit",
            },
          },
        },
      },
      edges: [
        {
          source: "payments-api",
          target: "payments-db",
          socket: "databases",
        },
      ],
    },
    precalculated_paths: [
      {
        path_id: "path-0001",
        nodes: ["payments-api", "payments-db"],
        attached_features: ["feature/observability"],
      },
    ],
  };

  const serialised = JSON.parse(JSON.stringify(fixture));
  const ok = validate(serialised);
  assert.equal(ok, true, `schema validation failed: ${JSON.stringify(validate.errors)}`);

  const round = JSON.parse(JSON.stringify(fixture)) as PolicyValidatorInput;
  assert.deepStrictEqual(round, fixture);
});

test("PolicyValidatorOutput round-trips through wasm-policy-validator schema", () => {
  const validate = makeValidator("wasm-policy-validator-v1alpha1.schema.json");

  const fixture: PolicyValidatorOutput = {
    results: [
      { pass: true, message: "all labels present" },
      { pass: false, message: "version floor violation: payments-db < 15.0.0" },
    ],
  };

  const serialised = JSON.parse(JSON.stringify(fixture));
  const ok = validate(serialised);
  assert.equal(ok, true, `schema validation failed: ${JSON.stringify(validate.errors)}`);

  const round = JSON.parse(JSON.stringify(fixture)) as PolicyValidatorOutput;
  assert.deepStrictEqual(round, fixture);
});
