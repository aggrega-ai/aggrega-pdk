// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Fixture read-source-probe plugin for the Aggrega Wasm runtime,
// authored with the Go PDK and built with the standard Go toolchain
// for GOOS=wasip1.
//
// It exercises the aggrega_read_source_file host function end-to-end
// as a content-based source selection: the probe export reads an index
// file whose content names a second file, then reads that named file
// and returns its bytes. The selection of the second file is driven by
// the first file's content — the exact pattern the content-read
// callback exists to support. On any read failure (including a sandbox
// rejection of a path escaping the source root) it returns the in-band
// failure message as its output, so a host-side test can assert the
// failure surface without special-casing the call.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o read-source-probe-go.wasm ./
package main

import (
	"strings"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

//go:wasmexport probe
func probe() int32 {
	indexPath := string(pdk.Input())

	indexContent, err := aggrega.ReadSourceFile(indexPath)
	if err != nil {
		pdk.OutputString(err.Error())
		return 0
	}

	// The index file's content selects which file to read next — a
	// content-dependent selection the guest could not make from the
	// name-only listing alone.
	chosen := strings.TrimSpace(string(indexContent))
	payload, err := aggrega.ReadSourceFile(chosen)
	if err != nil {
		pdk.OutputString(err.Error())
		return 0
	}
	pdk.Output(payload)
	return 0
}

func main() {}
