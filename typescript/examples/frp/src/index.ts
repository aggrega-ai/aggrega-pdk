// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Fixture feature-recipe-pipeline step plugin for the Aggrega Wasm
 * runtime.
 *
 * It exercises the real pipeline contract end-to-end:
 *
 *  - It content-selects a workload from the keyed document set (a document
 *    whose kind is Deployment), learns that document's key at runtime, and
 *    roots an own-scope PATCH at it — adding an annotation.
 *  - It EMITs a new own-scope ConfigMap document (a pipeline side effect).
 *  - It EMITs a new host-scope marker document (an additive cross-entity
 *    side effect) only when the host document set is present — a
 *    component-scope FeatureRecipe attached to a host.
 *  - It RETURNS its `with` input as its outputs, entry for entry and in the
 *    order the entries arrived, so a later step reads each value through
 *    .steps.<id>.outputs.<key>.
 *
 * It self-describes via describe() as a pipeline-step plugin whose step export
 * declares operates=patch, scope=own, and the helm and helm-chart
 * compatible providers. The host reads describe() once at load, validates
 * the descriptor, then dispatches step with the keyed document set as
 * input.
 *
 * Entry convention follows the Extism JS PDK: parameterless function, expose
 * via module.exports. The pipeline-step socket carries canonical CBOR in both
 * directions, so step reads Host.inputBytes() into decodeCBORFRPStepInput and
 * writes its output with encodeCBORFRPStepOutput through Host.outputBytes().
 * The step codec is consumed from the TypeScript Plugin Development Kit, which
 * re-exports it from the portable ABI package.
 *
 * The step input and the outputs are Maps, which keep their entries in the
 * order they were set — an integer-like key included, which a plain object
 * would move to the front. Building the outputs as a Map is what lets a step
 * return them in the order it chose.
 */

import {
  API_VERSION,
  decodeCBORFRPStepInput,
  encodeCBORFRPStepOutput,
  encodeDescriptor,
  type JSONPatchOp,
  type PluginDescriptor,
  type SideEffect,
} from "@aggrega-ai/aggrega-pdk-typescript";

// The static describe() self-report: a pipeline-step plugin whose step
// export patches own-scope workloads and is tested-compatible with the helm
// and helm-chart providers (both render workloads carrying a pod template,
// in flat-document and chart-source form respectively). The host reads this
// once at load and validates it before invoking step.
function descriptor(): PluginDescriptor {
  return {
    abi_version: API_VERSION,
    plugin_type: "pipeline-step",
    description:
      "Patch a selected workload and emit companion documents in one feature-recipe pipeline step.",
    capabilities: ["helm", "helm-chart"],
    exports: [
      {
        name: "step",
        operates: "patch",
        scope: "own",
        selector: "workloads carrying a pod template",
      },
    ],
    metadata: { author: "aggrega", license: "Apache-2.0" },
  };
}

// treeKind reports the kind field of a unit's tree, or "" when the unit is
// opaque (no tree) or carries no kind.
function treeKind(tree: unknown): string {
  if (tree !== null && typeof tree === "object") {
    const kind = (tree as Record<string, unknown>).kind;
    if (typeof kind === "string") {
      return kind;
    }
  }
  return "";
}

// The host calls describe() once at load to read the plugin's static
// self-report before dispatching step.
function describe(): void {
  Host.outputString(encodeDescriptor(descriptor()));
}

function step(): void {
  const input = decodeCBORFRPStepInput(new Uint8Array(Host.inputBytes()));

  const patches: JSONPatchOp[] = [];
  const sideEffects: SideEffect[] = [];

  // Content-select a Deployment in the own set and annotate it. The guest
  // learns the unit's key at runtime — it never knows emitted filenames
  // ahead of time — and roots its patch at that key. It reads the unit's
  // tree (skipping opaque units that carry none) and inspects content.
  // The own set arrives as a Map, in the order the host holds the
  // documents; iterate its keys in sorted order so the patches come out in
  // one fixed order whatever that order is.
  const own = input.documents.own;
  const ownKeys = Array.from(own.keys()).sort();
  for (const key of ownKeys) {
    if (treeKind(own.get(key)?.tree) === "Deployment") {
      patches.push({
        scope: "own",
        key,
        op: "add",
        // RFC 6901 escapes `/` in the annotation key as `~1`.
        path: "/metadata/annotations/aggrega.ai~1pipeline-patched",
        value: "true",
      });
    }
  }

  // Own-scope EMIT: a new ConfigMap document the pipeline contributes.
  sideEffects.push({
    kind: "emit_file",
    scope: "own",
    file: {
      path: "pipeline-configmap.yaml",
      content:
        "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: pipeline-emitted\ndata:\n  source: frp-step-typescript\n",
    },
  });

  // Host-scope EMIT: an additive marker the pipeline contributes to the
  // host component's set — only when a host set is present (a component-
  // scope FeatureRecipe attached to a host). A Recipe step has no host
  // scope, so the guest does not emit one. Additive emits run per-node for
  // every provider.
  if (input.documents.host !== undefined && input.documents.host.size > 0) {
    sideEffects.push({
      kind: "emit_file",
      scope: "host",
      file: {
        path: "pipeline-host-marker.yaml",
        content:
          "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: pipeline-host-marker\ndata:\n  injected-by: frp-step-typescript\n",
      },
    });
  }

  // Return the `with` input as the step's outputs, in the order its entries
  // arrived.
  const outputs = new Map<string, unknown>();
  for (const [key, value] of input.step_input ?? new Map<string, unknown>()) {
    outputs.set(key, value);
  }

  Host.outputBytes(
    encodeCBORFRPStepOutput({ patches, side_effects: sideEffects, outputs }).buffer as ArrayBuffer,
  );
}

module.exports = { step, describe };
