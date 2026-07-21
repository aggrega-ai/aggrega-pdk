// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Fixture parse-source-probe plugin for the Aggrega Wasm runtime, authored with
// the Go PDK and built with the standard Go toolchain for GOOS=wasip1.
//
// It exercises the aggrega_parse_source host function end-to-end as a consumer
// of host-mediated producer output: the import export hands its input bytes to
// the host as a source to parse, and the host runs a parser for the requested
// format through the host and returns the canonical tree. On success the export
// outputs the tree bytes verbatim; on any failure — no parser for the format, a
// parse failure, or a denied re-entrant producer-consumption call — it outputs
// the in-band failure message prefixed with "ERR:", so a host-side test can
// assert the failure surface without special-casing the call.
//
// It self-describes as an importer plugin whose import export declares the
// aggrega_parse_source host function, so the load-time cross-check of the
// descriptor's declared host functions against the binary's imports covers the
// new host function.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o parse-source-probe-go.wasm ./
package main

import (
	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// descriptor is the static describe() self-report: a v1alpha1 importer plugin
// whose import export imports the aggrega_parse_source host function. The host
// reads this once at load, validates it, and cross-checks the declared host
// functions against the binary's imports.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "importer",
		Description: "Probe the aggrega_parse_source host function end-to-end.",
		Exports: []aggrega.PluginExport{{
			Name:          "import",
			HostFunctions: []string{"aggrega_parse_source"},
		}},
		Metadata: &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
}

//go:wasmexport describe
func describe() int32 {
	out, err := aggrega.MarshalDescriptor(descriptor())
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(out)
	return 0
}

//go:wasmexport import
func importSource() int32 {
	source := pdk.Input()

	// Obtain the source's canonical tree through the host — the host resolves a
	// parser for the format, runs it, and returns the tree — rather than parsing
	// the source in the guest.
	tree, err := aggrega.ParseSource("json", source, nil)
	if err != nil {
		pdk.OutputString("ERR:" + err.Error())
		return 0
	}
	pdk.Output(tree)
	return 0
}

func main() {}
