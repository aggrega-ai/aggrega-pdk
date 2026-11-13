// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Fixture fetch-probe plugin for the Aggrega Wasm runtime, authored
// with the Go PDK and built with the standard Go toolchain for
// GOOS=wasip1.
//
// It exercises the aggrega_fetch_external_input host function
// end-to-end: the probe export reads an external-input URL from its
// input, fetches that URL's bytes through the PDK wrapper, and returns
// the fetched bytes as its output. On a fetch failure it returns the
// failure message text as its output instead, so a host-side test can
// assert the in-band failure surface (the message naming the URL and
// the remedy) without special-casing the call.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o fetch-probe-go.wasm ./
package main

import (
	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

//go:wasmexport probe
func probe() int32 {
	url := string(pdk.Input())
	payload, err := aggrega.FetchExternalInput(url)
	if err != nil {
		pdk.OutputString(err.Error())
		return 0
	}
	pdk.Output(payload)
	return 0
}

func main() {}
