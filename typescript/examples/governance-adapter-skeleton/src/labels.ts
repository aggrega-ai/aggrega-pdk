// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * The label-transform role.
 *
 * What arrives: the build's nodes keyed by name, each carrying the component it
 * instantiates, the version resolved for it and the labels it currently has;
 * plus the deployment's name and environment, so a policy can decide differently
 * per environment.
 *
 * What may be emitted: label changes, one per decision. Every instruction this
 * module builds addresses one label on one node, so a decision has no way to
 * reach the node's component, its resolved version, the set of nodes, or a value
 * that is not a string. That confinement is the whole of what an operator invited
 * by attaching a plugin here.
 *
 * A policy names the label; this module builds the path. The change is expressed
 * to the build as a document of edit instructions addressed by path, and those
 * paths need escaping — a label key routinely contains a `/`, which is the path
 * separator, so a hand-written path would address a nested location and the change
 * would be refused. The escaping is written once, here, rather than in every
 * policy.
 *
 * How it travels: this role's wire carries the binary encoding in both
 * directions, so the payload is read and the result written through the generated
 * codec on the payload type. The seam reads a JSON document, so the decoded
 * payload is written back out as one before it is handed over — the names a policy
 * addresses are the payload's own names either way.
 */

import {
  decodeCBORMutatorInput,
  encodeCBORMutatorOutput,
  type MutatorOutput,
} from "@aggrega-ai/aggrega-pdk-typescript";

import { evaluate, type Program } from "./evaluator";
import { expectObject } from "./shape";

/**
 * The decision this role asks the engine for. See the annotation role for why
 * the name is the adapter's to choose and is stated once.
 */
export const DECISION = "labels";

/** The two operations a label decision may ask for. */
const OP_ADD = "add";
const OP_REMOVE = "remove";

const ROLE = "label-transform";

/** One label decision as a policy expresses it. */
interface Decision {
  /** The name of the node the decision applies to. */
  node: string;
  /** The label key the decision is about. */
  key: string;
  /** The value the label takes. Required when adding, unused when dropping. */
  value?: string;
  /** Whether the label is added or dropped. Absent means added. */
  op?: string;
}

/** One edit instruction, in the shape the build reads. */
interface Instruction {
  op: string;
  path: string;
  value?: string;
}

/** Reads the payload and returns the label changes the build applies, encoded. */
export function changes(raw: Uint8Array): Uint8Array {
  const input = decodeCBORMutatorInput(raw);
  expectObject(input.graph, "graph", ROLE);
  expectObject(input.graph.nodes, "graph.nodes", ROLE);

  const programs: Program[] = (input.sources ?? []).map((s) => ({
    path: s.path,
    text: s.content,
  }));

  const decisions = evaluate(programs, input.settings, JSON.stringify(input), DECISION);
  const patches = decisions.map((decision) => instruction(decision as Partial<Decision>));

  const out: MutatorOutput = { patches };
  return encodeCBORMutatorOutput(out);
}

/** Builds the single-operation edit document one decision asks for. */
function instruction(decision: Partial<Decision>): Instruction[] {
  if (typeof decision.node !== "string" || typeof decision.key !== "string") {
    throw new Error(
      `a decision collected under "${DECISION}" is not a label decision. ` +
        `A decision names "node", "key" and "value", and may name "op" as ` +
        `"${OP_ADD}" or "${OP_REMOVE}".`,
    );
  }
  const op = decision.op ?? OP_ADD;
  let value: string | undefined;
  if (op === OP_ADD) {
    if (typeof decision.value !== "string" || decision.value === "") {
      throw new Error(
        `the "${DECISION}" decision for node "${decision.node}" label ` +
          `"${decision.key}" names no "value"; a label value is a string`,
      );
    }
    value = decision.value;
  } else if (op !== OP_REMOVE) {
    throw new Error(
      `the "${DECISION}" decision for node "${decision.node}" label ` +
        `"${decision.key}" names "op" "${op}", which is not one of ` +
        `"${OP_ADD}" or "${OP_REMOVE}"`,
    );
  }
  const path = `/nodes/${escapeSegment(decision.node)}/labels/${escapeSegment(decision.key)}`;
  return value === undefined ? [{ op, path }] : [{ op, path, value }];
}

/**
 * Escapes one path segment so a name containing the path separator addresses a
 * single location rather than a nested one.
 *
 * The escape character is itself escaped first, so a separator rewritten into an
 * escape sequence is not escaped twice.
 */
function escapeSegment(segment: string): string {
  return segment.replace(/~/g, "~0").replace(/\//g, "~1");
}
