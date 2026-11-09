// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Fixture parse-source-probe plugin for the Aggrega Wasm runtime, authored with
// the Go PDK and built with the standard Go toolchain for GOOS=wasip1.
//
// It exercises the aggrega_parse_source host function end-to-end as a consumer
// of host-mediated producer output: the import export decodes the ImportInput it
// is handed — canonical CBOR, as the importer socket carries it — and hands the
// primary source bytes to the host as a source to parse, and the host runs a
// parser for the requested format through the host and returns the canonical
// tree. It answers the way the importer socket reads an answer, an ImportOutput
// in canonical CBOR: on success one scaffold file, treeFile, whose content is the
// tree bytes verbatim; on any failure — an input that does not decode, no parser
// for the format, a parse failure, or a denied re-entrant producer-consumption
// call — the in-band failure message in the output's error field, so a host-side
// test can assert the failure surface without special-casing the call.
//
// It self-describes as an importer plugin whose import export declares the
// aggrega_parse_source host function, so the load-time cross-check of the
// descriptor's declared host functions against the binary's imports covers the
// new host function.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o parse-source-probe-go.wasm ./
package main

import (
	"fmt"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// descriptor is the static describe() self-report: an importer plugin
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

// treeFile is the path of the one scaffold file the probe answers with on
// success; its content is the tree the host returned, verbatim.
const treeFile = "tree.json"

//go:wasmexport import
func importSource() int32 {
	var in aggrega.ImportInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		return answer(aggrega.ImportOutput{Error: "decode ImportInput: " + err.Error()})
	}

	// Obtain the source's canonical tree through the host — the host resolves a
	// parser for the format, runs it, and returns the tree — rather than parsing
	// the source in the guest.
	tree, err := aggrega.ParseSource("json", in.Bytes, nil)
	if err != nil {
		return answer(aggrega.ImportOutput{Error: err.Error()})
	}
	return answer(aggrega.ImportOutput{Files: []aggrega.ScaffoldFile{{Path: treeFile, Content: string(tree)}}})
}

// answer writes out in the canonical CBOR the importer socket carries. Only an
// output the codec cannot encode is reported out of band, because there is then
// no answer to carry a message in.
func answer(out aggrega.ImportOutput) int32 {
	raw, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("parse-source-probe: encode ImportOutput: %w", err))
		return 1
	}
	pdk.Output(raw)
	return 0
}

func main() {}
