// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

/**
 * The ir-verdict and render-verdict roles, one export each.
 *
 * The two exports share this one implementation because the two payloads carry
 * the same two members the seam needs. Which of them arrived is the caller's to
 * say — the export the host dispatched names the role — so each payload is read
 * as the shape its own role carries rather than guessed at from its content.
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
  decodeCBORPolicyInput,
  decodeCBORRenderPolicyInput,
  encodeCBORPolicyOutput,
  encodePolicyInput,
  encodeRenderPolicyInput,
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
 * What the two verdict payloads have in common once decoded: the programs the
 * attachment supplied, its settings, and the whole payload as the JSON document
 * the seam evaluates against.
 */
interface Payload {
  sources: PolicySource[];
  settings: unknown;
  document: string;
}

/**
 * Reads one verdict role's payload and returns the verdicts the build reports,
 * encoded.
 */
export function verdicts(raw: Uint8Array, role: string): Uint8Array {
  const payload = readPayload(raw, role);
  const programs: Program[] = payload.sources.map((s) => ({
    path: s.path,
    text: s.content,
  }));

  const results: Diagnostic[] = [];
  for (const [decision, severity] of COLLECTIONS) {
    for (const value of evaluate(programs, payload.settings, payload.document, decision)) {
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
  return encodeCBORPolicyOutput(out);
}

/**
 * Reads one verdict role's payload.
 *
 * The two roles carry different payloads and the same two members out of them,
 * which is why one reader serves both. Which payload to expect is the caller's to
 * say, so this reads the shape it was told to and nothing else: a payload that
 * does not read that way is refused rather than tried again as the other role's
 * shape and quietly found to contain nothing.
 *
 * The payload is read through the generated codec and written back out as the
 * JSON document the seam reads. The graph's order-keeping members arrive as Maps,
 * which JSON.stringify writes as empty objects, so the document is written through
 * the generated encoder, which writes each Map as an object in the order it holds.
 */
function readPayload(raw: Uint8Array, role: string): Payload {
  if (role === "render-verdict") {
    const input = decodeCBORRenderPolicyInput(raw);
    expectList(input.artifacts, "artifacts", "render-verdict");
    return {
      sources: input.sources ?? [],
      settings: input.settings,
      document: encodeRenderPolicyInput(input),
    };
  }

  const input = decodeCBORPolicyInput(raw);
  expectObject(input.graph, "graph", "ir-verdict");
  expectList(input.precalculated_paths, "precalculated_paths", "ir-verdict");
  return {
    sources: input.sources ?? [],
    settings: input.settings,
    document: encodePolicyInput(input),
  };
}
