// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//go:build !tinygo

package aggrega

// Log is a no-op on non-Wasm builds. The TinyGo build at host_funcs.go
// declares the //go:wasmimport binding that calls into the Aggrega
// host's log function. Unit tests on the host can import the PDK and
// reference Log without a Wasm runtime; the no-op stub keeps the
// signature stable across both build paths.
func Log(level int, message string) {
	_ = level
	_ = message
}

// EmitWarning is a no-op on non-Wasm builds. See [Log] for the build
// asymmetry rationale.
func EmitWarning(code, message, path string) {
	_ = code
	_ = message
	_ = path
}
