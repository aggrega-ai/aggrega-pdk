// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

/**
 * The annotation-transform role.
 *
 * What arrives: the components this attachment may annotate, each with the
 * boundary it sits under, its own name and the annotations it already carries;
 * plus the annotation keys the Manifest has registered a meaning for. An
 * annotation under one of those keys goes on to expand into an attached feature,
 * so a policy reads that list to inject keys that will take effect.
 *
 * What may be emitted: annotations to add to components that already exist. A
 * decision carries a coordinate, a key and a value — a string, or an object of
 * subkeys for a binding that routes several — and there is no other field it
 * could carry, so this role cannot express any other change. A decision naming a
 * component the payload does not carry, or carrying a value of another shape, is
 * refused by the build, naming the coordinate. The value's shape is the build's
 * to judge, so it is passed through as the policy wrote it.
 *
 * A decision is read straight into the wire's own injection shape, so the field
 * names a policy writes are the field names the build reads.
 */

import {
  decodeCBORAnnotationTransformInput,
  encodeAnnotationTransformInput,
  encodeCBORAnnotationTransformOutput,
  type AnnotationInjection,
  type AnnotationTransformOutput,
} from "@aggrega-ai/aggrega-pdk-typescript";

import { evaluate, type Program } from "./evaluator";
import { expectList } from "./shape";

/**
 * The decision this role asks the engine for.
 *
 * The name is the adapter's own: the engine behind the seam is told which
 * decision to return, and this is the word a policy author collects annotation
 * decisions under. Change it and the policies written against this adapter change
 * with it, so it is stated once, here.
 */
export const DECISION = "annotations";

const ROLE = "annotation-transform";

/** Reads the payload and returns the annotations the build adds, encoded. */
export function injections(raw: Uint8Array): Uint8Array {
  const input = decodeCBORAnnotationTransformInput(raw);
  expectList(input.targets, "targets", ROLE);

  const programs: Program[] = (input.sources ?? []).map((s) => ({
    path: s.path,
    text: s.content,
  }));

  // Each target's annotations arrive as a Map, which JSON.stringify writes as an
  // empty object, so the payload is written out through the generated encoder: it
  // writes each Map as an object in the order the Map holds.
  const decisions = evaluate(programs, input.settings, encodeAnnotationTransformInput(input), DECISION);

  const emitted: AnnotationInjection[] = decisions.map((decision) => {
    const injection = decision as Partial<AnnotationInjection>;
    if (
      typeof injection.boundary_path !== "string" ||
      typeof injection.component_name !== "string" ||
      typeof injection.annotation_key !== "string" ||
      injection.annotation_value === undefined
    ) {
      throw new Error(
        `a decision collected under "${DECISION}" is not an annotation decision. ` +
          `A decision names "boundary_path", "component_name", "annotation_key" and "annotation_value".`,
      );
    }
    if (injection.annotation_key === "" || injection.component_name === "") {
      throw new Error(
        `a decision collected under "${DECISION}" leaves "annotation_key" or ` +
          `"component_name" empty; an annotation with no key, or none to put it on, ` +
          `is not something a policy meant to ask for`,
      );
    }
    return injection as AnnotationInjection;
  });

  const out: AnnotationTransformOutput = { injections: emitted };
  return encodeCBORAnnotationTransformOutput(out);
}
