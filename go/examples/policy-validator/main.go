// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Fixture policy-validator plugin for the Aggrega Wasm runtime.
//
// Emits a deterministic two-result PolicyValidatorOutput: one
// pass=true result and one pass=false result. The fixture does not
// inspect the input graph; the outputs are constants so end-to-end
// tests can assert exact equality of the emitted shape.
//
// Build: tinygo build -target=wasm-unknown -o policy-validator-go.wasm ./
package main

import (
	pdk "github.com/extism/go-pdk"
)

// The fixture emits a constant two-result PolicyValidatorOutput. The
// output is hand-formatted rather than marshalled through
// encoding/json: TinyGo's wasm-unknown runtime has incomplete
// reflection support, which causes encoding/json to trap on struct
// marshalling. Hand-formatting keeps the fixture working under the
// current TinyGo target and produces the exact wire shape the
// wasm-policy-validator JSON Schema describes. The PDK's own
// round-trip tests run under the host's standard Go toolchain and
// exercise the full encoding/json path against the same schema.
const fixtureOutput = `{"results":[` +
	`{"pass":true,"message":"fixture rule A: ok"},` +
	`{"pass":false,"message":"fixture rule B: deliberate fail for end-to-end coverage"}` +
	`]}`

//export validate
func validate() int32 {
	pdk.OutputString(fixtureOutput)
	return 0
}

func main() {}
