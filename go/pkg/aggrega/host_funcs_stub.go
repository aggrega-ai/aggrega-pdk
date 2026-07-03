// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//go:build !wasip1

package aggrega

import "fmt"

// Log is a no-op off Wasm. The wasip1 build at host_funcs.go declares
// the //go:wasmimport binding that calls into the Aggrega host's log
// function. Unit tests on the host can import the PDK and reference Log
// without a Wasm runtime; the no-op stub keeps the signature stable
// across both build paths.
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

// FetchExternalInput returns a not-available error off Wasm. Unlike the
// void diagnostic emitters, a fetch has a data result, so the off-Wasm
// stub cannot be a silent no-op; the error keeps the failure loud in
// host-side unit tests that reach it. The wasip1 build at
// host_funcs.go declares the //go:wasmimport binding that calls into
// the Aggrega host's fetch function.
func FetchExternalInput(url string) ([]byte, error) {
	return nil, fmt.Errorf("aggrega_fetch_external_input: not available outside the Wasm runtime (requested %q)", url)
}
