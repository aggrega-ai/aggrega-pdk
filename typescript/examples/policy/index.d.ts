// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Interface declaration consumed by extism-js at compile time.

declare module "extism:host" {
  interface user {}
}

declare module "main" {
  export function verdict_ir(): I32;
  export function describe(): I32;
}
