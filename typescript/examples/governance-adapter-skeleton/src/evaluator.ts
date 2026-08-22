// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * The seam: the one module a new adapter rewrites.
 */

import { AUDIT_DECISION } from "./verdicts";

/**
 * One policy program the attachment carried.
 *
 * It is the four roles' source entries normalized to one shape, so the engine
 * behind the seam reads a single type rather than four that differ only in their
 * declaring role.
 */
export interface Program {
  /**
   * The file's base name, as the attachment carried it. The host supplies a base
   * name and never a path from its own filesystem, so the same attachment yields
   * the same programs wherever it is built.
   */
  path: string;
  /**
   * The program's source text, carried verbatim. Nothing outside this guest has
   * read it.
   */
  text: string;
}

/**
 * Runs the policy engine this adapter wraps and returns the decisions it reached
 * under one named decision.
 *
 * Replace this body with a call into your engine; everything else in this
 * skeleton can stay as it is.
 *
 * The four arguments are the four things an adapter maps:
 *
 * - `programs` is the attachment's files, which are the engine's program text.
 * - `settings` is the attachment's settings object. It is the engine's data, and
 *   the place to read any tunable the adapter itself needs. It is `undefined`
 *   when the attachment declares no settings.
 * - `payload` is the whole role payload as JSON text, which is the document the
 *   engine evaluates against. Handing over the whole payload rather than a
 *   projection of it means a policy reads the graph, the artifacts, the nodes or
 *   the targets under the names the wire already gives them, so the input is
 *   learnable from one place. A role whose wire carries the binary encoding writes
 *   the decoded payload back out as JSON before handing it over, so the seam sees
 *   one document format at every role and a policy addresses the same names
 *   whichever encoding delivered them.
 * - `decision` names which decision the caller wants back. Each role asks for its
 *   own; the names are the constants declared in the role modules.
 *
 * Each returned element is one decision. The role that asked for it reads it into
 * the shape that role emits, so the field names a policy author writes are the
 * field names the wire carries and there is no second spelling to keep in step.
 *
 * Two obligations on whatever replaces this body:
 *
 * Identical input must yield an identical sequence of decisions. A build's output
 * is byte-identical across runs of identical input, and an adapter that iterated
 * an unordered collection would break that from inside the guest. Where an engine
 * yields a set, sort it before returning.
 *
 * A failure to evaluate must throw, never return an empty result. An empty result
 * is a policy that found nothing to say; a thrown failure is a policy that did
 * not run. Reporting the second as the first lets a build through unexamined.
 *
 * This body reaches no engine and reads no program. It reports back what it was
 * handed, so a freshly built adapter says something at every role rather than
 * being indistinguishable from one that is silently doing nothing: the two
 * transform roles emit no changes, and the verdict roles emit one `info` note.
 * That note is the wiring proof, and it goes with the placeholder.
 */
export function evaluate(
  programs: Program[],
  settings: unknown,
  payload: string,
  decision: string,
): unknown[] {
  void payload;

  if (decision !== AUDIT_DECISION) {
    return [];
  }
  const received =
    programs.length === 0
      ? "no programs"
      : programs.map((p) => `${p.path} (${p.text.length} bytes)`).join(", ");
  return [
    `this adapter has no policy engine yet: the seam received ${received} and ` +
      `${settings === undefined ? "no" : "some"} settings`,
  ];
}
