// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Interface declaration consumed by extism-js at compile time. It names the
// exports the four governance roles dispatch to; an adapter serving fewer roles
// drops the ones it does not implement here and in the describe() self-report.

declare module "extism:host" {
  interface user {}
}

declare module "main" {
  export function describe(): I32;
  export function transform_annotations(): I32;
  export function mutate(): I32;
  export function verdict_ir(): I32;
  export function verdict_render(): I32;
}
