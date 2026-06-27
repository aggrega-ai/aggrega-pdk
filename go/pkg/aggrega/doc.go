// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Package aggrega is the Go PDK for authoring Aggrega Wasm plugins.
//
// The package exports the per-socket payload types plugin authors
// marshal to and from JSON, the [APIVersion] contract floor constant,
// and ergonomic wrappers over the Aggrega host functions (Log,
// EmitWarning). The host-function bindings compile under the standard
// Go GOOS=wasip1 target; off Wasm the bindings are no-op stubs so the
// library compiles cleanly for unit tests and tooling.
//
// See the package's README for plugin-authoring examples and the
// per-socket usage shape.
package aggrega
