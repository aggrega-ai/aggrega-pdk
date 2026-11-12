// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

/**
 * Refusing a payload this guest cannot read.
 *
 * The generated decode functions build a typed VIEW of the decoded payload rather
 * than validating it: TypeScript's types are erased before the guest runs, and a
 * decode does not check that a member the shape requires arrived. A role
 * therefore checks the member it depends on itself.
 *
 * The alternative is a role that reads `undefined` where it expected a list,
 * iterates nothing, and emits an empty result — which reports "the policy found
 * nothing to say" for a payload it never understood. A guest that cannot read its
 * input says so.
 */

/** Refuses a payload whose named member is not a list. */
export function expectList(value: unknown, field: string, role: string): void {
  if (!Array.isArray(value)) {
    throw new Error(`reading the ${role} payload: "${field}" is not a list`);
  }
}

/** Refuses a payload whose named member is not an object. */
export function expectObject(value: unknown, field: string, role: string): void {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error(`reading the ${role} payload: "${field}" is not an object`);
  }
}
