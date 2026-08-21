// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * The ir-verdict and render-verdict roles, on one `validate` export.
 *
 * One export serves both because the payload identifies itself by shape: the
 * render-verdict payload carries the rendered output files, the ir-verdict
 * payload carries the build graph. Reading the shape is what lets this guest
 * serve both without declaring which role it is attached at — and the role, not
 * the guest, is what decides which payload arrives.
 *
 * What arrives at the ir-verdict role: the build graph, and the routing paths
 * enumerated alongside it so a policy can reason about what reaches what without
 * walking the graph itself.
 *
 * What arrives at the render-verdict role: the rendered output files, each with
 * its output path, its rendered text, the format it was rendered in, and — where
 * the host parsed the content before handing it over — a structured view of that
 * content, so a policy can reason about a document it does not read natively. A
 * file the host did not parse carries no structured view and is read as text.
 *
 * What may be emitted: verdicts, and nothing else. A verdict role reports an
 * outcome; the build reads it as an outcome and never applies it as a change. An
 * empty result means no verdict fired, which is a policy that allowed the build.
 *
 * Decision collections are not wire severities. `deny`, `warn` and `audit` are
 * decision collections — a convention many policy languages already group
 * decisions under, adopted here so a policy author can reuse what they know. An
 * adapter is free to pick other names, or to read them from the attachment's
 * settings so an operator can choose. The severities the wire carries are a
 * separate, closed set of three: `halt` stops the build and names what to fix,
 * `warn` surfaces alongside a build that still completes, and `info` records the
 * observation and leaves the build's outcome unchanged. COLLECTIONS is the
 * mapping between the two, and keeping them apart is what lets a policy author
 * rename a decision collection without changing what the build does with it.
 */

import {
  SeverityHalt,
  SeverityInfo,
  SeverityWarn,
  decodePolicyInput,
  decodeRenderPolicyInput,
  encodePolicyOutput,
  type Diagnostic,
  type PolicyOutput,
  type PolicySource,
} from "@aggrega-ai/aggrega-pdk-typescript";

import { evaluate, type Program } from "./evaluator";
import { expectList, expectObject } from "./shape";

/** The decisions the verdict roles ask the engine for. */
export const DENY_DECISION = "deny";
export const WARN_DECISION = "warn";
export const AUDIT_DECISION = "audit";

/**
 * Each decision collection paired with the severity this adapter reports it as,
 * in the order the results are emitted.
 */
export const COLLECTIONS: ReadonlyArray<readonly [string, string]> = [
  [DENY_DECISION, SeverityHalt],
  [WARN_DECISION, SeverityWarn],
  [AUDIT_DECISION, SeverityInfo],
];

/**
 * What the two verdict payloads have in common once the facet is known: the role
 * that delivered it and the programs and settings it carried.
 */
interface Facet {
  role: string;
  sources: PolicySource[];
  settings: unknown;
}

/** Reads the payload and returns the verdicts the build reports, encoded. */
export function verdicts(raw: string): string {
  const facet = readFacet(raw);
  const programs: Program[] = facet.sources.map((s) => ({
    path: s.path,
    text: s.content,
  }));

  const results: Diagnostic[] = [];
  for (const [decision, severity] of COLLECTIONS) {
    for (const value of evaluate(programs, facet.settings, raw, decision)) {
      if (typeof value !== "string") {
        throw new Error(
          `a decision collected under "${decision}" is not a message. ` +
            `A verdict decision is the text the build reports.`,
        );
      }
      results.push({ severity, message: value });
    }
  }

  const out: PolicyOutput = { results };
  return encodePolicyOutput(out);
}

/**
 * Reads the payload and reports which verdict role delivered it.
 *
 * Which facet arrived is read from the payload rather than assumed, so a payload
 * this guest cannot read is refused instead of being evaluated as the other facet
 * and quietly finding nothing.
 */
function readFacet(raw: string): Facet {
  const probe: unknown = JSON.parse(raw);
  expectObject(probe, "the payload", "verdict");

  if ("artifacts" in (probe as Record<string, unknown>)) {
    const input = decodeRenderPolicyInput(raw);
    expectList(input.artifacts, "artifacts", "render-verdict");
    return {
      role: "render-verdict",
      sources: input.sources ?? [],
      settings: input.settings,
    };
  }

  const input = decodePolicyInput(raw);
  expectObject(input.graph, "graph", "ir-verdict");
  expectList(input.precalculated_paths, "precalculated_paths", "ir-verdict");
  return {
    role: "ir-verdict",
    sources: input.sources ?? [],
    settings: input.settings,
  };
}
