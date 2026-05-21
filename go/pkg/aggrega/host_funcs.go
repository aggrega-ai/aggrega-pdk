// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//go:build tinygo

package aggrega

import (
	pdk "github.com/extism/go-pdk"
)

// Log emits a structured log message to the Aggrega host. Level is the
// severity (0=trace, 1=debug, 2=info, 3=warn, 4=error per the project's
// log-level integer convention). Message is the log line.
//
// This is the TinyGo build of the binding. The standard-host build at
// host_funcs_stub.go provides a no-op so unit tests on the host can
// import the PDK without a Wasm runtime.
func Log(level int, message string) {
	mem := pdk.AllocateString(message)
	defer mem.Free()
	aggregaLog(uint64(level), uint64(mem.Offset()))
}

// EmitWarning emits a structured warning the host's build-time
// validator surfaces via the warning channel. Code is a short
// identifier the validator categorises by; message is the warning
// text; path is an optional JSON-Pointer-style location into the
// authored entity (e.g., "components.payments-api.values.memory").
//
// This is the TinyGo build of the binding.
func EmitWarning(code, message, path string) {
	codeMem := pdk.AllocateString(code)
	defer codeMem.Free()
	msgMem := pdk.AllocateString(message)
	defer msgMem.Free()
	pathMem := pdk.AllocateString(path)
	defer pathMem.Free()
	aggregaEmitWarning(uint64(codeMem.Offset()), uint64(msgMem.Offset()), uint64(pathMem.Offset()))
}

// aggregaLog is the host-imported binding for the Aggrega log host
// function. Signature matches the host-side declaration at
// aggrega-cli/internal/wasm/hostfuncs.go: (level i64, message_ptr i64).
//
//go:wasmimport extism:host/user aggrega_log
func aggregaLog(level uint64, messagePtr uint64)

// aggregaEmitWarning is the host-imported binding for the Aggrega
// warning-emit host function. Signature: (code_ptr i64, message_ptr i64,
// path_ptr i64).
//
//go:wasmimport extism:host/user aggrega_emit_warning
func aggregaEmitWarning(codePtr uint64, messagePtr uint64, pathPtr uint64)
