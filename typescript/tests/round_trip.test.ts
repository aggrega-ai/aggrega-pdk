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
import { readFileSync, readdirSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import Ajv from "ajv/dist/2020.js";

import {
  API_VERSION,
  type CLIRunInput,
  encodeCLIRunInput,
  decodeCLIRunInput,
  type MutatorInput,
  type MutatorOutput,
  type PolicyInput,
  type PolicyOutput,
} from "../src/index.ts";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// makeValidator compiles the named schema with every schema in the sibling
// aggrega-schemas json directory registered alongside it. The registration set is
// derived by reading that directory, so a cross-file $ref resolves without a
// network loader and without any per-caller list of referenced schemas to keep in
// step with the schemas themselves — an unregistered ref target does not produce a
// wrong answer, it aborts compilation and masks every assertion that follows.
//
// Each schema registers under its $id when it declares one, falling back to its
// file name: a relative cross-file $ref resolves against the referring schema's
// base URI, which is its absolute $id URL, so the target must be reachable under
// that URL rather than under its file name. The named schema is retrieved by the
// same identity it was registered under.
function makeValidator(schemaName: string) {
  const schemaDir = join(__dirname, "..", "..", "..", "aggrega-schemas", "json");
  if (!existsSync(schemaDir)) {
    throw new Error(
      `schemas not reachable at ${schemaDir}: this test requires ` +
        `the umbrella checkout layout (aggrega-pdk/ and aggrega-schemas/ ` +
        `as siblings). Skip with --test-skip-pattern=round_trip.`,
    );
  }

  const ajv = new Ajv({ strict: false });
  let schemaKey: string | undefined;
  for (const name of readdirSync(schemaDir)
    .filter((n) => n.endsWith(".schema.json"))
    .sort()) {
    const schema = JSON.parse(
      readFileSync(join(schemaDir, name), "utf-8"),
    ) as { $id?: string };
    const id =
      typeof schema.$id === "string" && schema.$id !== "" ? schema.$id : name;
    ajv.addSchema(schema, id);
    if (name === schemaName) {
      schemaKey = id;
    }
  }
  if (schemaKey === undefined) {
    throw new Error(`schema ${schemaName} not found in ${schemaDir}`);
  }

  const validate = ajv.getSchema(schemaKey);
  if (validate === undefined) {
    throw new Error(`schema ${schemaName} not resolvable as ${schemaKey}`);
  }
  return validate;
}

test("API_VERSION is the v1alpha1 contract floor", () => {
  assert.equal(API_VERSION, "v1alpha1");
});

test("CLIRunInput round-trips through wasm-cli schema", () => {
  const validate = makeValidator("wasm-cli-v1alpha1.schema.json");

  const fixture: CLIRunInput = { command: "deploy", args: ["deploy", "--env", "prod", "payments"] };

  const wire = encodeCLIRunInput(fixture);
  const ok = validate(JSON.parse(wire));
  assert.equal(ok, true, `schema validation failed: ${JSON.stringify(validate.errors)}`);

  const round = decodeCLIRunInput(wire);
  assert.deepStrictEqual(round, fixture);
});

test("MutatorInput round-trips through wasm-mutator schema", () => {
  const validate = makeValidator("wasm-mutator-v1alpha1.schema.json");

  const fixture: MutatorInput = {
    compilation_context: {
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

test("PolicyInput round-trips through wasm-policy schema", () => {
  // The policy input carries the full IR graph, and the policy schema references
  // ir-v1alpha1 for that graph field and wasm-diagnostic-v1alpha1 for its result
  // entries; both resolve because the whole schema directory is registered.
  const validate = makeValidator("wasm-policy-v1alpha1.schema.json");

  const fixture: PolicyInput = {
    graph: {
      apiVersion: "aggrega.ai/v1alpha1",
      kind: "IR",
      metadata: { aggregate_name: "payments" },
      spec: {
        nodes: {
          "payments-api": {
            component: "core/web-service",
            resolved_version: "1.4.2",
            labels: { team: "payments" },
          },
        },
        edges: [
          {
            source: "payments-api",
            target: "payments-db",
            socket: "databases",
            socket_declaration: "declared",
          },
        ],
        feature_markers: [],
        values_mapping: {},
        infra_mapping: {},
        feature_values_mapping: {},
        provenance: {
          schema_version: "v1alpha1",
          compiler_version: "test",
          aggregate_version: "1.0.0",
          manifest_version: "1.0.0",
          release_version: "1.0.0",
          source_identifiers: ["x"],
        },
      },
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

  const round = JSON.parse(JSON.stringify(fixture)) as PolicyInput;
  assert.deepStrictEqual(round, fixture);
});

test("PolicyOutput round-trips through wasm-policy schema", () => {
  const validate = makeValidator("wasm-policy-v1alpha1.schema.json");

  const fixture: PolicyOutput = {
    results: [
      { severity: "deny", message: "version floor violation: payments-db < 15.0.0" },
      { severity: "audit", message: "all labels present" },
    ],
  };

  const serialised = JSON.parse(JSON.stringify(fixture));
  const ok = validate(serialised);
  assert.equal(ok, true, `schema validation failed: ${JSON.stringify(validate.errors)}`);

  const round = JSON.parse(JSON.stringify(fixture)) as PolicyOutput;
  assert.deepStrictEqual(round, fixture);
});
