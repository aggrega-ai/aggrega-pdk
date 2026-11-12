// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

/**
 * Adapter skeleton for a governance plugin: copy this directory, drop your policy
 * engine into the seam, and the result is a guest an operator can attach at every
 * governance role.
 *
 * This is a skeleton, not a framework. No policy engine is compiled in and no
 * policy language is assumed. Everything specific to an engine lives behind one
 * function, `evaluate` in evaluator.ts, which is the one module a new adapter
 * has to rewrite. The role modules around it — annotations.ts, labels.ts,
 * verdicts.ts — are the parts that stay the same whatever engine is behind the
 * seam: each decodes the payload its role receives, hands it to the seam, and
 * encodes the output its role is allowed to emit.
 *
 * # The four roles this guest serves
 *
 * A governance plugin is one guest attached at one of four roles, and the role an
 * operator attaches it at — not anything the guest says about itself — fixes which
 * payload arrives and what the guest may emit. All four are served here:
 *
 *   annotation-transform  transform_annotations  the components that may be annotated
 *   label-transform       mutate                 the build's nodes and their labels
 *   ir-verdict            verdict_ir             the build graph
 *   render-verdict        verdict_render         the rendered output files
 *
 * Each role has its own export, so the export the host dispatches is what says
 * which role the guest is serving and which payload it is reading. The two
 * verdict exports share one implementation, because the two verdict payloads
 * carry the same two members the seam needs.
 *
 * # The four things an adapter maps
 *
 * Whatever engine sits behind the seam, an adapter is wiring four things:
 *
 * - sources become the engine's program text. The attachment's files arrive
 *   verbatim; the host does not parse them.
 * - settings become the engine's data, and any translation the adapter itself
 *   needs. The attachment's settings object arrives verbatim too.
 * - the input is the payload of the attached role, listed in the table above.
 * - the output is either the verdicts the engine reached, or the changes a
 *   transform role asks for.
 *
 * # How a failure travels
 *
 * A role that cannot read its payload, or whose engine could not evaluate, throws,
 * and the build reads that as a failed invocation. That is one of two channels: the
 * other is the error member every role's output carries, for an adapter that can
 * distinguish "evaluated, and here is what went wrong" from "could not run at all"
 * and wants the first reported alongside a partial result. This skeleton takes one
 * channel rather than both, because two paths to one outcome is two places to keep
 * in step. What neither channel permits is reporting a failure as an empty result,
 * which on a governance surface reads as a policy that allowed the build.
 *
 * Entry convention follows the Extism JS PDK: parameterless functions, input read
 * from and output written to the host buffers, exports exposed through
 * module.exports. Every role's wire carries the binary encoding in both
 * directions, so every role reads and writes bytes.
 */

import {
  API_VERSION,
  encodeDescriptor,
  type PluginDescriptor,
} from "@aggrega-ai/aggrega-pdk-typescript";

import { injections } from "./annotations";
import { changes } from "./labels";
import { verdicts } from "./verdicts";

/**
 * The self-report the host reads once when it loads the guest.
 *
 * It declares one export per governance role. The host refuses to call an export
 * the guest did not declare, so an export is declared here in the same change
 * that implements it — an attachment pointed at a missing export is then refused
 * by name rather than silently doing nothing.
 *
 * An adapter serving fewer roles declares fewer exports here, drops them from
 * index.d.ts and module.exports, and deletes the role modules for the roles it does not serve.
 * It declares no capabilities: a governance plugin is not selected by capability,
 * and what it may emit comes from its role.
 *
 * settings_schema is left unset, so the attachment's settings pass through
 * verbatim. An adapter whose engine needs a settings shape can declare a JSON
 * Schema document there; the host then validates the operator's settings against
 * it before dispatch, and settings that do not conform halt the build.
 */
function descriptor(): PluginDescriptor {
  return {
    abi_version: API_VERSION,
    plugin_type: "governance",
    description: "Adapter skeleton: wire a policy engine to the governance roles.",
    exports: [
      {
        name: "transform_annotations",
        description: "Report the annotations the build's components should carry.",
      },
      { name: "mutate", description: "Report the labels the build's nodes should carry." },
      {
        name: "verdict_ir",
        description: "Report the verdicts the policy reached over the build graph.",
      },
      {
        name: "verdict_render",
        description: "Report the verdicts the policy reached over the rendered output.",
      },
    ],
    metadata: { license: "Apache-2.0" },
  };
}

function describe(): void {
  Host.outputString(encodeDescriptor(descriptor()));
}

/** Serves the annotation-transform role. */
function transform_annotations(): void {
  Host.outputBytes(injections(new Uint8Array(Host.inputBytes())).buffer as ArrayBuffer);
}

/** Serves the label-transform role. */
function mutate(): void {
  Host.outputBytes(changes(new Uint8Array(Host.inputBytes())).buffer as ArrayBuffer);
}

/** Serves the ir-verdict role. */
function verdict_ir(): void {
  Host.outputBytes(verdicts(new Uint8Array(Host.inputBytes()), "ir-verdict").buffer as ArrayBuffer);
}

/** Serves the render-verdict role. */
function verdict_render(): void {
  Host.outputBytes(
    verdicts(new Uint8Array(Host.inputBytes()), "render-verdict").buffer as ArrayBuffer,
  );
}

module.exports = { describe, transform_annotations, mutate, verdict_ir, verdict_render };
