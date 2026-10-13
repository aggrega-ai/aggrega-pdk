// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Flowise importer plugin for the Aggrega Wasm runtime, authored with the Go
// PDK and built with the standard Go toolchain for GOOS=wasip1.
//
// An importer turns a foreign infrastructure description into Aggrega entities so
// a consumer can adopt existing infrastructure without hand-authoring the entities
// from scratch. This importer reads Flowise flow JSON — chatflows and
// agentflows — and drafts an Aggregate composing the flow's nodes as components,
// referencing Components of the workspace's catalog where a node matches one with
// high confidence and drafting new components where none does.
//
// The importer holds only what is particular to Flowise: this descriptor and the
// export wiring, the walker lifting a flow's nodes and edges (walk.go), and the
// format's mapping arm and syntactic format (format.go). The rest — the
// confidence scorer, the decisions, the drafts, the renderers and the settings —
// is the kit's importer package, github.com/aggrega-ai/aggrega-pdk-go/pkg/importer,
// which every importer built on this kit shares. All of it runs in this guest:
// the host mediates the pipeline and stays blind to what a node means. The
// scorer weighs each node against the Components of the workspace's own catalog,
// which the host hands the guest on its input as a view of their names, versions,
// labels, key names and sockets; the mapping arm is carried in the guest, and
// operators extend or override it through the import attachment's settings.
//
// It obtains each source's structured tree through the host rather than parsing
// the source itself: the import export hands the source bytes to the
// aggrega_parse_source host function for the JSON syntactic format and walks the
// canonical tree the host returns. So it declares the aggrega_parse_source host
// function on its import export, and the load-time cross-check of the descriptor's
// declared host functions against the binary's imports covers it.
//
// The import payload is decoded and the result encoded through the generated ABI
// codec (github.com/aggrega-ai/aggrega-abi-go/pkg/abi, re-exported by this PDK)
// rather than hand-declared wire types, so the wire shape stays single-sourced
// from the schema. The importer socket carries canonical CBOR in both directions:
// the input is read with DecodeCBOR and the output is written with EncodeCBOR.
//
// Build: ./build.sh, which stamps the version the guest reports through
// describe() (AGGREGA_PLUGIN_VERSION, default 1.0.0) into
// GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -ldflags "-X github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega.Version=<version>" -o import-flowise-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
	"github.com/aggrega-ai/aggrega-pdk-go/pkg/importer"
)

// descriptor is the static describe() self-report: a v1alpha1 importer plugin
// serving the flowise agent-flow format, whose import export imports the
// aggrega_parse_source host function it uses to obtain each source's tree, and
// the signature a Flowise source is recognised by (format.go). The host reads
// this once at load, validates it, and cross-checks the declared host functions
// against the binary's imports and the signatures against the ones the
// importer's Plugin entity records.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:   aggrega.APIVersion,
		PluginType:   "importer",
		Description:  "Import Flowise agent-flow JSON into a draft Aggregate, referencing Components of the workspace's catalog where a node matches and drafting new components where none does.",
		Capabilities: []string{"import:" + flowise.Name},
		Exports: []aggrega.PluginExport{{
			Name:          "import",
			HostFunctions: []string{"aggrega_parse_source"},
		}},
		Importer: &aggrega.ImporterContract{Detect: detect},
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

// importArtifact is the import export. The Go function is named differently from
// the export because "import" is a Go keyword; the //go:wasmexport directive
// carries the export name the host dispatches.
//
//go:wasmexport import
func importArtifact() int32 {
	var in aggrega.ImportInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(fmt.Errorf("import-flowise: decode ImportInput: %w", err))
		return 1
	}

	// The parse callback is the guest's only channel to a source tree: it asks the
	// host to run a parser for the syntactic format and hands back the canonical
	// tree. The guest never parses the source with a grammar of its own.
	out := importer.Import(in, flowise, func(syntax string, source []byte) (json.RawMessage, error) {
		return aggrega.ParseSource(syntax, source, nil)
	})

	raw, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("import-flowise: encode ImportOutput: %w", err))
		return 1
	}
	pdk.Output(raw)
	return 0
}

func main() {}
