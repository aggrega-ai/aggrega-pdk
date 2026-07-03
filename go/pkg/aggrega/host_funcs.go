// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//go:build wasip1

package aggrega

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	pdk "github.com/extism/go-pdk"
)

// Log emits a structured log message to the Aggrega host. Level is the
// severity (0=trace, 1=debug, 2=info, 3=warn, 4=error per the project's
// log-level integer convention). Message is the log line.
//
// This is the wasip1 build of the binding. The off-Wasm build at
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
// This is the wasip1 build of the binding.
func EmitWarning(code, message, path string) {
	codeMem := pdk.AllocateString(code)
	defer codeMem.Free()
	msgMem := pdk.AllocateString(message)
	defer msgMem.Free()
	pathMem := pdk.AllocateString(path)
	defer pathMem.Free()
	aggregaEmitWarning(uint64(codeMem.Offset()), uint64(msgMem.Offset()), uint64(pathMem.Offset()))
}

// FetchExternalInput fetches the bytes of an external-input URL
// through the Aggrega host's acquisition channel. The host serves the
// URL under its external-input pin discipline: an already-pinned URL
// is served from the content-addressed store; an unpinned URL is
// fetched and pinned when acquisition is enabled, and refused with a
// clear message when the closure is frozen. The returned bytes are the
// input's payload.
//
// Failure is in-band: the host returns an error envelope rather than
// trapping, and this wrapper surfaces it as the returned error — its
// message names the URL and the remedy. Guests should treat a fetch
// error as a render failure to report, not a condition to mask.
//
// This is the wasip1 build of the binding; the off-Wasm build at
// host_funcs_stub.go returns a not-available error.
func FetchExternalInput(url string) ([]byte, error) {
	mem := pdk.AllocateString(url)
	defer mem.Free()
	offset := aggregaFetchExternalInput(uint64(mem.Offset()))
	if offset == 0 {
		return nil, errors.New("aggrega_fetch_external_input: host returned no envelope")
	}
	envMem := pdk.FindMemory(offset)
	envBytes := envMem.ReadBytes()
	envMem.Free()

	var envelope struct {
		Content *string `json:"content"`
		Error   string  `json:"error"`
	}
	if err := json.Unmarshal(envBytes, &envelope); err != nil {
		return nil, fmt.Errorf("aggrega_fetch_external_input: decode envelope: %w", err)
	}
	if envelope.Error != "" {
		return nil, errors.New(envelope.Error)
	}
	if envelope.Content == nil {
		return nil, errors.New("aggrega_fetch_external_input: envelope carries neither content nor error")
	}
	payload, err := base64.StdEncoding.DecodeString(*envelope.Content)
	if err != nil {
		return nil, fmt.Errorf("aggrega_fetch_external_input: decode content: %w", err)
	}
	return payload, nil
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

// aggregaFetchExternalInput is the host-imported binding for the
// Aggrega external-input fetch host function. Signature:
// (url_ptr i64) -> envelope_ptr i64; the returned offset addresses one
// JSON envelope in plugin memory carrying either a base64 content
// field or an error field, and offset 0 means no envelope was
// delivered.
//
//go:wasmimport extism:host/user aggrega_fetch_external_input
func aggregaFetchExternalInput(urlPtr uint64) uint64
