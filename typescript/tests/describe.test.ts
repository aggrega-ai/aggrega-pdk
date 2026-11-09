// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Descriptor-builder tests for the Aggrega TypeScript PDK.
 *
 * `encodeDescriptor` marshals a typed PluginDescriptor to the wire bytes the
 * host reads once at load, omitting every absent optional field, and folds the
 * build-provenance stamp (`AGGREGA_PLUGIN_VERSION` / `AGGREGA_PLUGIN_COMMIT`,
 * injected by esbuild `--define`) into the descriptor's metadata. The stamp path
 * is exercised here by setting the constants on `globalThis` before the call —
 * the same `typeof` guard esbuild's `--define` substitution flows through.
 */

import { test } from "node:test";
import { strict as assert } from "node:assert";

import {
  encodeDescriptor,
  type PluginDescriptor,
} from "../src/index.ts";

test("encodeDescriptor omits absent optionals and carries command flags", () => {
  const d: PluginDescriptor = {
    abi_version: "v1beta1",
    plugin_type: "cli",
    exports: [
      { name: "cli_run", host_functions: ["aggrega_cli_write_stdout"] },
    ],
    commands: [
      {
        name: "echo-args",
        description: "Echo the received argument vector to standard output.",
        flags: [
          {
            name: "upper",
            description: "Echo the arguments upper-cased.",
            type: "bool",
            default: "false",
          },
        ],
      },
    ],
    metadata: { author: "aggrega", license: "Apache-2.0" },
  };
  const wire = encodeDescriptor(d);
  for (const key of [
    '"contract"',
    '"stage"',
    '"settings_schema"',
    '"capabilities"',
    '"version"',
    '"commit"',
  ]) {
    assert.ok(!wire.includes(key), `absent optional ${key} leaked: ${wire}`);
  }
  for (const want of ['"commands"', '"flags"', '"upper"', '"bool"']) {
    assert.ok(wire.includes(want), `expected ${want} in wire: ${wire}`);
  }
});

test("encodeDescriptor emits a full provider contract and round-trips", () => {
  const d: PluginDescriptor = {
    abi_version: "v1beta1",
    plugin_type: "provider",
    description: "Render one IR node and assemble the dispatch group.",
    capabilities: ["helm"],
    exports: [
      { name: "render_node", input_pattern: "b", stability: "experimental" },
      { name: "assemble_boundary", input_pattern: "b" },
    ],
    contract: {
      input_kinds: { sourced: true, synthesized: true, artifacts_standalone: false },
      boundary_assembly: "merge",
      artifact_delimiters: { left: "[[", right: "]]" },
      assembly_name_annotation: {
        annotation: "aggrega.ai/release-name",
        require_unique: true,
      },
      reusable_instance: true,
    },
    metadata: { author: "aggrega", license: "Apache-2.0" },
  };
  const wire = encodeDescriptor(d);
  for (const want of [
    '"contract"',
    '"boundary_assembly":"merge"',
    '"artifact_delimiters"',
    '"[["',
    '"assembly_name_annotation"',
    '"require_unique":true',
    '"reusable_instance":true',
  ]) {
    assert.ok(wire.includes(want), `expected ${want} in wire: ${wire}`);
  }

  // Round-trip: decode the wire and re-encode; the bytes are stable.
  const decoded = JSON.parse(wire) as PluginDescriptor;
  assert.equal(encodeDescriptor(decoded), wire);
});

test("encodeDescriptor carries typed metadata version and commit fields", () => {
  const d: PluginDescriptor = {
    abi_version: "v1beta1",
    plugin_type: "provider",
    exports: [{ name: "render_node", input_pattern: "b" }],
    metadata: {
      author: "aggrega",
      license: "Apache-2.0",
      version: "1.2.3",
      commit: "abc1234",
    },
  };
  const wire = encodeDescriptor(d);
  assert.ok(wire.includes('"version":"1.2.3"'), wire);
  assert.ok(wire.includes('"commit":"abc1234"'), wire);
});

test("encodeDescriptor folds the build-provenance stamp into metadata", () => {
  const g = globalThis as Record<string, unknown>;
  g.AGGREGA_PLUGIN_VERSION = "9.9.9";
  g.AGGREGA_PLUGIN_COMMIT = "deadbee";
  try {
    const d: PluginDescriptor = {
      abi_version: "v1beta1",
      plugin_type: "cli",
      exports: [{ name: "cli_run" }],
    };
    const wire = encodeDescriptor(d);
    assert.ok(
      wire.includes('"metadata":{"version":"9.9.9","commit":"deadbee"}'),
      `expected folded provenance metadata, got: ${wire}`,
    );
  } finally {
    delete g.AGGREGA_PLUGIN_VERSION;
    delete g.AGGREGA_PLUGIN_COMMIT;
  }

  // With the constants unset, an unstamped descriptor omits both members.
  const bare: PluginDescriptor = {
    abi_version: "v1beta1",
    plugin_type: "cli",
    exports: [{ name: "cli_run" }],
  };
  const wire = encodeDescriptor(bare);
  assert.ok(!wire.includes('"version"'), wire);
  assert.ok(!wire.includes('"commit"'), wire);
  assert.ok(!wire.includes('"metadata"'), wire);
});
