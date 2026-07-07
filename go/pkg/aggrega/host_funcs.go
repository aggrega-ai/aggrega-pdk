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

// PackSource asks the Aggrega host to pack a selection from the acquired
// source and returns the packed files. Op names the shape of the selection
// ("file", "dir", or "tree") and path names it relative to the source root
// (the empty string names the root itself). The host resolves the selection
// under the source root, refusing any path that escapes above it, and returns
// the packed bytes as a source projection.
//
// This is the guest side of the host-listing source model: the host hands the
// guest a name-only listing, the guest applies its own resolution logic, and
// this call is the guest's request that the host read and pack the chosen
// selection — the only point at which source bytes cross into the guest.
//
// Failure is in-band: the host returns an error envelope rather than trapping,
// and this wrapper surfaces it as the returned error. A guest should treat a
// pack failure as a render failure to report, not a condition to mask.
//
// This is the wasip1 build of the binding; the off-Wasm build at
// host_funcs_stub.go returns a not-available error.
func PackSource(op, path string) (*SourceProjection, error) {
	reqBytes, err := SourcePackRequest{Op: op, Path: path}.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("aggrega_pack_source: encode request: %w", err)
	}
	mem := pdk.AllocateString(string(reqBytes))
	defer mem.Free()
	offset := aggregaPackSource(uint64(mem.Offset()))
	if offset == 0 {
		return nil, errors.New("aggrega_pack_source: host returned no envelope")
	}
	respMem := pdk.FindMemory(offset)
	respBytes := respMem.ReadBytes()
	respMem.Free()

	var resp SourcePackResponse
	if err := resp.UnmarshalJSON(respBytes); err != nil {
		return nil, fmt.Errorf("aggrega_pack_source: decode response: %w", err)
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	if resp.SourceProjection == nil {
		return nil, errors.New("aggrega_pack_source: response carries neither a source projection nor an error")
	}
	return resp.SourceProjection, nil
}

// ReadSourceFile asks the Aggrega host to read a single file from the acquired
// source and returns its bytes. Path names the file relative to the source root.
// The host resolves the path under the source root, refusing any path that
// escapes above it, and returns the file's raw bytes.
//
// It is the content-based-selection primitive: a guest whose resolution logic
// must inspect a file's content before choosing what to pack reads that file
// through this call, then issues its PackSource request. No production guest
// uses it yet — the production guests resolve from the name-only listing alone —
// so it is exercised by the test-fixture guest rather than a shipped one.
//
// Failure is in-band, matching PackSource: the host returns an error envelope
// rather than trapping. This is the wasip1 build; the off-Wasm build at
// host_funcs_stub.go returns a not-available error.
func ReadSourceFile(path string) ([]byte, error) {
	reqBytes, err := SourceFileRequest{Path: path}.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("aggrega_read_source_file: encode request: %w", err)
	}
	mem := pdk.AllocateString(string(reqBytes))
	defer mem.Free()
	offset := aggregaReadSourceFile(uint64(mem.Offset()))
	if offset == 0 {
		return nil, errors.New("aggrega_read_source_file: host returned no envelope")
	}
	respMem := pdk.FindMemory(offset)
	respBytes := respMem.ReadBytes()
	respMem.Free()

	var resp SourceFileResponse
	if err := resp.UnmarshalJSON(respBytes); err != nil {
		return nil, fmt.Errorf("aggrega_read_source_file: decode response: %w", err)
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.Content, nil
}

// aggregaLog is the host-imported binding for the Aggrega log host
// function. Signature matches the host-side declaration at
// aggrega-cli/internal/wasm/hostfuncs.go: (level i64, message_ptr i64).
//
//go:wasmimport extism:host/user aggrega_log
func aggregaLog(level uint64, messagePtr uint64)

// aggregaPackSource is the host-imported binding for the Aggrega pack-source
// host function. Signature: (request_ptr i64) -> envelope_ptr i64; the argument
// addresses a SourcePackRequest JSON document in plugin memory and the returned
// offset addresses a SourcePackResponse JSON envelope carrying either a packed
// source projection or an error. Offset 0 means no envelope was delivered.
//
//go:wasmimport extism:host/user aggrega_pack_source
func aggregaPackSource(requestPtr uint64) uint64

// aggregaReadSourceFile is the host-imported binding for the Aggrega
// read-source-file host function. Signature: (request_ptr i64) -> envelope_ptr
// i64; the argument addresses a SourceFileRequest JSON document in plugin memory
// and the returned offset addresses a SourceFileResponse JSON envelope carrying
// either the file's base64 content or an error. Offset 0 means no envelope was
// delivered.
//
//go:wasmimport extism:host/user aggrega_read_source_file
func aggregaReadSourceFile(requestPtr uint64) uint64

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
