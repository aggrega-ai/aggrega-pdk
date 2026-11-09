// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Fixture provider plugin exercising the cross-boundary edge-linking
 * exports for the Aggrega Wasm runtime.
 *
 * It advertises cross_boundary_linking in describe() and offers both link
 * exports as the paired contract the host routes a cross-boundary uses
 * edge through:
 *
 *  - link_publish runs on the consumed (target) endpoint. It derives a
 *    deterministic pointer string from the edge, returns it as the
 *    published handle the consumer resolves against, and returns one
 *    structural patch that records the pointer on the consumed endpoint's
 *    assembled file.
 *  - link_resolve runs on the consuming (source) endpoint. It reads the
 *    published handle and returns one structural patch that injects the
 *    pointer into the consuming endpoint's assembled file.
 *
 * Each patch operation is authored as opaque JSON matching the shared
 * pipeline patch-operation wire shape (scope/key/op/path/value); the host
 * carries it verbatim and applies it through its codec. Output is a pure
 * function of the input, so identical input yields byte-identical output.
 *
 * The mandatory render_node export is present and minimal — the link pass
 * never invokes it, but a provider plugin must declare it.
 *
 * Entry convention follows the Extism JS PDK: parameterless function,
 * read input via Host.inputBytes(), write output via Host.outputBytes(),
 * expose via module.exports. The render socket's wire is canonical CBOR in
 * both directions, so each export reads its input and writes its output
 * through the link codec's binary entry points, consumed from the
 * TypeScript Plugin Development Kit, which re-exports them from the portable
 * ABI package.
 */

import {
  API_VERSION,
  decodeCBORLinkPublishInput,
  encodeDescriptor,
  encodeCBORLinkPublishOutput,
  decodeCBORLinkResolveInput,
  encodeCBORLinkResolveOutput,
  encodeCBORRenderNodeOutput,
  type LinkPatch,
  type PluginDescriptor,
} from "@aggrega-ai/aggrega-pdk-typescript";

// The assembled file each endpoint's dispatch group emits and this fixture
// wires. Both the consumed and consuming endpoints carry a file of this
// name in their own dispatch group; the two are distinct files in distinct
// groups.
const LINK_FILE = "boundary.yaml";

// The static describe() self-report: a provider plugin that
// declares the cross_boundary_linking capability and both link exports as a
// pair, alongside the mandatory render_node export.
function descriptor(): PluginDescriptor {
  const experimental = (name: string) => ({
    name,
    input_pattern: "b",
    stability: "experimental",
  });
  return {
    abi_version: API_VERSION,
    plugin_type: "provider",
    description:
      "Wire a cross-boundary uses edge by publishing a deterministic pointer and resolving it into each endpoint's assembled file.",
    capabilities: ["link-cross-boundary"],
    exports: [
      experimental("render_node"),
      experimental("link_publish"),
      experimental("link_resolve"),
    ],
    contract: {
      input_kinds: {
        sourced: false,
        synthesized: false,
        artifacts_standalone: true,
      },
      boundary_assembly: "none",
      cross_boundary_linking: true,
    },
    metadata: { author: "aggrega", license: "Apache-2.0" },
  };
}

// derivedPointer builds the deterministic pointer string a consumed
// endpoint publishes for one link: the target node id and the socket alias
// the edge is bound under, joined by a fixed separator. The compiler pipes
// it as an opaque scalar; the runtime environment resolves it at deploy
// time.
function derivedPointer(targetNode: string, socket: string): string {
  return `${targetNode}--${socket}`;
}

// oneOp authors a single structural patch operation matching the shared
// pipeline patch-operation wire shape. The host carries it verbatim; the
// field order is fixed so the emitted op is byte-identical across runs of
// identical input.
function oneOp(file: string, path: string, value: string): unknown {
  return { scope: "own", key: file, op: "add", path, value };
}

// The host calls describe() once at load to read the plugin's static
// self-report before dispatching any export.
function describe(): void {
  Host.outputString(encodeDescriptor(descriptor()));
}

function render_node(): void {
  // The link pass never routes render_node; a provider plugin must still
  // declare it. Emit nothing.
  Host.outputBytes(encodeCBORRenderNodeOutput({ files: [] }).buffer as ArrayBuffer);
}

function link_publish(): void {
  const input = decodeCBORLinkPublishInput(new Uint8Array(Host.inputBytes()));
  const pointer = derivedPointer(input.edge.target.node, input.edge.socket);

  const patches: LinkPatch[] = [
    { file: LINK_FILE, ops: [oneOp(LINK_FILE, "/published", pointer)] },
  ];

  Host.outputBytes(
    encodeCBORLinkPublishOutput({ handle: { ref: pointer }, patches }).buffer as ArrayBuffer,
  );
}

function link_resolve(): void {
  const input = decodeCBORLinkResolveInput(new Uint8Array(Host.inputBytes()));
  // Inject the peer's published pointer verbatim into the consuming
  // endpoint's assembled file.
  const pointer = input.handle.ref ?? "";

  const patches: LinkPatch[] = [
    { file: LINK_FILE, ops: [oneOp(LINK_FILE, "/resolved", pointer)] },
  ];

  Host.outputBytes(encodeCBORLinkResolveOutput({ patches }).buffer as ArrayBuffer);
}

module.exports = { render_node, link_publish, link_resolve, describe };
