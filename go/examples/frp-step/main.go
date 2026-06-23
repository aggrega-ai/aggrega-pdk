// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Fixture feature-recipe-pipeline step plugin for the Aggrega Wasm
// runtime. It self-describes via describe() as an frp_step plugin and
// implements a no-op step export.
//
// The fixture exists to exercise the host's shared plugin-load gate
// end-to-end: load the module, call describe(), validate the returned
// descriptor against the host contract, then dispatch the named export.
// Real step logic — AST patches, emitted side effects — is later work;
// this fixture proves the load-and-dispatch path, not pipeline
// behaviour.
//
// Build: tinygo build -target=wasm-unknown -o frp-step-go.wasm ./
package main

import pdk "github.com/extism/go-pdk"

// pluginDescriptor is the static describe() self-report: a v1alpha1
// frp_step plugin whose minimum export, step, is present. The host
// reads this once at load and validates it before invoking step.
const pluginDescriptor = `{"abi_version":"v1alpha1","plugin_type":"frp_step","exports":[{"name":"step"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}`

//export describe
func describe() int32 {
	pdk.Output([]byte(pluginDescriptor))
	return 0
}

//export step
func step() int32 {
	// No-op step: an empty FRP step output (no patches, no side
	// effects, no diagnostics). Enough to prove the gate dispatches the
	// declared export after describe() validation.
	pdk.Output([]byte(`{}`))
	return 0
}

func main() {}
