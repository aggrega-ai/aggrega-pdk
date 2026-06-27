// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Fixture policy-validator plugin for the Aggrega Wasm runtime.
//
// Emits a deterministic two-result PolicyValidatorOutput: one
// pass=true result and one pass=false result. The fixture does not
// inspect the input graph; the outputs are constants so end-to-end
// tests can assert exact equality of the emitted shape.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-validator-go.wasm ./
package main

import (
	pdk "github.com/extism/go-pdk"
)

// The fixture emits a constant two-result PolicyValidatorOutput. The
// output is a hand-formatted literal — the fixture's wire shape is
// fixed, so a constant string is the simplest faithful encoding and
// matches the exact wire shape the wasm-policy-validator JSON Schema
// describes. The PDK's own round-trip tests exercise the full
// encoding/json path against the same schema.
const fixtureOutput = `{"results":[` +
	`{"pass":true,"message":"fixture rule A: ok"},` +
	`{"pass":false,"message":"fixture rule B: deliberate fail for end-to-end coverage"}` +
	`]}`

//go:wasmexport validate
func validate() int32 {
	pdk.OutputString(fixtureOutput)
	return 0
}

func main() {}
