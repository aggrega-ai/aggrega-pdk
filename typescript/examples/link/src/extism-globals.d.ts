// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Globals the Extism plugin runtime provides that @extism/js-pdk's type
// package does not declare.
//
// A plugin's entry point exposes its exported functions through the CommonJS
// `module.exports` object, which the QuickJS runtime embedded in the compiled
// artifact supplies. @extism/js-pdk declares the Host, Memory and Var surfaces
// but not that object, so a plugin written to the documented entry convention
// has no declaration for it and does not type-check on its own configuration.
//
// Declared here, in the plugin's own sources, rather than by adding Node's
// type definitions: a plugin compiled to Wasm never runs on Node, and pulling
// that whole surface in to obtain one global would let a plugin reference APIs
// its runtime does not have.
//
// This file declares types only. The bundler follows imports from the entry
// module, so nothing here reaches the bundled artifact.

declare const module: { exports: Record<string, unknown> };
