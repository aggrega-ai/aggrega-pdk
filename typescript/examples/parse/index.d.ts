// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Interface declaration consumed by extism-js at compile time.
// The `extism:host` user interface is empty because the fixture does
// not invoke any Aggrega host functions — it decodes a ParseInput and
// encodes a ParseOutput over the payload the host hands it; the `main`
// module declares the plugin's exported functions for extism-js to bind.

declare module "extism:host" {
  interface user {}
}

declare module "main" {
  export function parse(): I32;
  export function describe(): I32;
}
