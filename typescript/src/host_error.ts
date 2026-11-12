// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

/**
 * The error a host-function wrapper throws when the host reports an in-band
 * failure. It extends Error, so `error.message` carries the free-form human
 * message and a consumer that only logs or rethrows is unaffected; the added
 * `code` is the machine-matchable identity a consumer branches on:
 *
 * ```ts
 * try {
 *   packSource("file", "../etc/passwd");
 * } catch (e) {
 *   if (e instanceof HostError && e.code === CodeSourceEscape) {
 *     // handle a sandbox-escape refusal specifically
 *   }
 * }
 * ```
 *
 * Absent- and unknown-code tolerance is built in: an older host that sends no
 * code leaves `code` empty, and a newer host that sends a code this build
 * predates leaves `code` set to that unrecognized value — either way a compare
 * against a known `Code*` constant does not match and the consumer falls back to
 * the message, with no thrown surprise beyond the failure itself.
 */
export class HostError extends Error {
  readonly code: string;

  constructor(code: string, message: string) {
    super(message);
    this.name = "HostError";
    this.code = code;
  }
}
