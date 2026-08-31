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
 * `../../aggrega-schemas/json/` relative to this crate root. A run that could
 * not read them has verified nothing, so these tests refuse with a clear
 * diagnostic rather than passing quietly. Where the schemas are genuinely
 * absent — a standalone checkout of this kit, or a packaging test — opt out
 * with `AGGREGA_SKIP_SCHEMA_TESTS=1`, and each affected test reports that it
 * was skipped and why.
 */

import { test, type TestContext } from "node:test";
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
  type NameBoundInput,
  type NameBoundOutput,
  encodeNameBoundInput,
  encodeNameBoundOutput,
  decodeNameBoundOutput,
  type MutatorInput,
  type MutatorOutput,
  type PolicyInput,
  type PolicyOutput,
} from "../src/index.ts";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// Names the environment switch that opts out of the tests which validate
// against the JSON Schemas published alongside this kit. Set it to any
// non-empty value where those schemas are genuinely absent — a standalone
// checkout of this kit, or a packaging test — and each affected test reports
// that it was skipped and why.
const SKIP_SCHEMA_TESTS_ENV = "AGGREGA_SKIP_SCHEMA_TESTS";

const schemaDir = join(__dirname, "..", "..", "..", "aggrega-schemas", "json");

// schemasUnreachableReason returns the reason the named JSON Schema cannot be
// read, or undefined when it can. This is the one place that decides
// reachability, so every test reaches the same verdict on the same condition.
function schemasUnreachableReason(schemaName: string): string | undefined {
  if (!existsSync(schemaDir)) {
    return `schemas not reachable at ${schemaDir}`;
  }
  const path = join(schemaDir, schemaName);
  if (!existsSync(path)) {
    return `schema ${schemaName} not reachable at ${path}`;
  }
  return undefined;
}

// skipSchemaTests reports whether the opt-out switch is set to a non-empty
// value. A value is not parsed: any non-empty setting opts out, so no spelling
// of "yes" can be read as "no" and quietly leave the check running.
function skipSchemaTests(): boolean {
  const value = process.env[SKIP_SCHEMA_TESTS_ENV];
  return typeof value === "string" && value !== "";
}

// requireSchemas ends the calling test when the named JSON Schema cannot be
// read, and reports whether the caller should return immediately.
//
// The default is refusal. A harness that could not reach its inputs has
// verified nothing, so reporting that is strictly more informative than passing
// quietly — and a suite that skips by default cannot tell "everything agrees"
// apart from "nothing was checked".
//
// Setting AGGREGA_SKIP_SCHEMA_TESTS turns the refusal into a skip that states
// the reason it was taken, so a deliberate run without the schemas stays clean
// while an accidental one stays loud.
function requireSchemas(t: TestContext, schemaName: string): boolean {
  const reason = schemasUnreachableReason(schemaName);
  if (reason === undefined) {
    return false;
  }
  if (skipSchemaTests()) {
    t.skip(`${reason}; ${SKIP_SCHEMA_TESTS_ENV} is set`);
    return true;
  }
  throw new Error(
    `${reason}: these tests validate against the JSON Schemas published ` +
      `alongside this kit and verify nothing without them. Restore the umbrella ` +
      `checkout layout (aggrega-pdk/ and aggrega-schemas/ as siblings), or set ` +
      `${SKIP_SCHEMA_TESTS_ENV}=1 to skip them.`,
  );
}

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
// same identity it was registered under. Reachability is settled by
// requireSchemas before a test gets here.
function makeValidator(schemaName: string) {
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

// schemaDeclaredABIVersion reads the plugin ABI version out of the descriptor
// schema in the sibling aggrega-schemas repository — the single-valued const the
// schema declares for a descriptor's abi_version. Reading the declaration rather
// than repeating its value is the whole point: a literal here would pin this
// kit's constant against a second copy living in this same repository, which is
// the divergence that generating the constant from the schema exists to remove.
function schemaDeclaredABIVersion(): string {
  const path = join(schemaDir, "wasm-describe-v1alpha1.schema.json");
  const doc = JSON.parse(readFileSync(path, "utf8"));
  const declared = doc?.$defs?.PluginDescriptor?.properties?.abi_version?.const;
  // An absent read means the declaration moved, not that the two versions agree.
  // Throwing keeps a silent undefined from being compared as though it matched.
  if (typeof declared !== "string" || declared === "") {
    throw new Error(
      `no abi_version const found at $defs.PluginDescriptor.properties in ${path}: ` +
        `the declaration moved, so this assertion is no longer reading it`,
    );
  }
  return declared;
}

// Checks this kit's exported constant against the schema that declares the
// version, which is the one artifact upstream of every language's copy.
test("API_VERSION matches the abi_version the descriptor schema declares", (t) => {
  if (requireSchemas(t, "wasm-describe-v1alpha1.schema.json")) return;

  assert.equal(
    API_VERSION,
    schemaDeclaredABIVersion(),
    "this constant is generated from the schema declaration, so a mismatch means " +
      "the generated const is stale — regenerate it",
  );
});

test("CLIRunInput round-trips through wasm-cli schema", (t) => {
  if (requireSchemas(t, "wasm-cli-v1alpha1.schema.json")) return;

  const validate = makeValidator("wasm-cli-v1alpha1.schema.json");

  const fixture: CLIRunInput = { command: "deploy", args: ["deploy", "--env", "prod", "payments"] };

  const wire = encodeCLIRunInput(fixture);
  const ok = validate(JSON.parse(wire));
  assert.equal(ok, true, `schema validation failed: ${JSON.stringify(validate.errors)}`);

  const round = decodeCLIRunInput(wire);
  assert.deepStrictEqual(round, fixture);
});

test("MutatorInput round-trips through wasm-mutator schema", (t) => {
  if (requireSchemas(t, "wasm-mutator-v1alpha1.schema.json")) return;

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

test("MutatorOutput round-trips through wasm-mutator schema", (t) => {
  if (requireSchemas(t, "wasm-mutator-v1alpha1.schema.json")) return;

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

test("PolicyInput round-trips through wasm-policy schema", (t) => {
  if (requireSchemas(t, "wasm-policy-v1alpha1.schema.json")) return;

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

test("PolicyOutput round-trips through wasm-policy schema", (t) => {
  if (requireSchemas(t, "wasm-policy-v1alpha1.schema.json")) return;

  const validate = makeValidator("wasm-policy-v1alpha1.schema.json");

  const fixture: PolicyOutput = {
    results: [
      { severity: "halt", message: "version floor violation: payments-db < 15.0.0" },
      { severity: "info", message: "all labels present" },
    ],
  };

  const serialised = JSON.parse(JSON.stringify(fixture));
  const ok = validate(serialised);
  assert.equal(ok, true, `schema validation failed: ${JSON.stringify(validate.errors)}`);

  const round = JSON.parse(JSON.stringify(fixture)) as PolicyOutput;
  assert.deepStrictEqual(round, fixture);
});

test("NameBoundOutput keeps an absent bound distinct from a bound of zero", (t) => {
  if (requireSchemas(t, "wasm-provider-v1alpha1.schema.json")) return;

  const validate = makeValidator("wasm-provider-v1alpha1.schema.json");

  // The three states the bound can be in. Absent says length is not what the
  // target objected to — either it imposes none, or the objection is one no
  // shortening repairs. Zero says no identity of any length reaches an
  // acceptable name. A host unable to tell them apart would narrow a name
  // toward a bound that was never the obstacle.
  const fixture: NameBoundOutput = {
    names: [
      { kind: "release name", usable: false, bound: 31, reason: "a release name is at most 53 characters" },
      { kind: "logical id", usable: false, bound: 0, reason: "this target accepts no name of this kind" },
      { kind: "service name", usable: false, reason: "the first character is one the alphabet does not admit" },
    ],
  };

  const wire = encodeNameBoundOutput(fixture);
  const ok = validate(JSON.parse(wire));
  assert.equal(ok, true, `schema validation failed: ${JSON.stringify(validate.errors)}`);

  // The emitted text is what is asserted, not only the decoded value: a bound
  // written as a plain number would emit 0 for the absent case and nothing
  // downstream could recover the difference.
  assert.equal(wire.includes('"bound":0'), true, `a zero bound must reach the wire as a stated length; got ${wire}`);

  const round = decodeNameBoundOutput(wire);
  assert.equal(round.names?.[0].bound, 31);
  assert.equal(round.names?.[1].bound, 0, "a zero bound must survive as a stated length");
  assert.equal(round.names?.[2].bound, undefined, "an absent bound must survive as absent, not as zero");
});

test("NameBoundInput round-trips through the wasm-provider schema", (t) => {
  if (requireSchemas(t, "wasm-provider-v1alpha1.schema.json")) return;

  const validate = makeValidator("wasm-provider-v1alpha1.schema.json");

  const fixture: NameBoundInput = { proposed: "payments-api", authored: true };

  const wire = encodeNameBoundInput(fixture);
  const ok = validate(JSON.parse(wire));
  assert.equal(ok, true, `schema validation failed: ${JSON.stringify(validate.errors)}`);

  const round = JSON.parse(wire) as NameBoundInput;
  assert.equal(round.proposed, fixture.proposed);
  assert.equal(round.authored, true);
});
