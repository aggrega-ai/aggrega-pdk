// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// All-pass proof policy-validator plugin for the Aggrega Wasm runtime.
//
// Emits a deterministic single-result PolicyValidatorOutput whose only
// verdict passes, so the host-side end-to-end test can prove the Step-4
// policy socket lets a build succeed when every rule passes — the
// counterpart to the minimal policy-validator fixture, whose deliberate
// failing verdict proves the build-halt path.
//
// It self-describes via describe() as a policy-validator plugin whose
// validate export the host validates before dispatch.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o policy-validator-allow-go.wasm ./
package main

import (
	pdk "github.com/extism/go-pdk"
)

// fixtureOutput is the constant all-pass verdict. The plugin does not
// inspect the input graph; the output is a fixed literal so the
// end-to-end test asserts the build-succeeds path deterministically.
const fixtureOutput = `{"results":[{"pass":true,"message":"proof allow: every rule passed"}]}`

// pluginDescriptor is the static describe() self-report: a v1alpha1
// policy-validator plugin whose validate export the host validates before
// dispatch.
const pluginDescriptor = `{"abi_version":"v1alpha1","plugin_type":"policy-validator","exports":[{"name":"validate"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}`

//go:wasmexport describe
func describe() int32 {
	pdk.Output([]byte(pluginDescriptor))
	return 0
}

//go:wasmexport validate
func validate() int32 {
	pdk.OutputString(fixtureOutput)
	return 0
}

func main() {}
