// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Fixture provider render plugin for the Aggrega Wasm runtime, authored
// with the Go PDK and built with the standard Go toolchain for
// GOOS=wasip1.
//
// Applies one trivial, deterministic transform — emits a single output
// file at a fixed path whose bytes are "rendered-node:" followed by the
// node id — and returns it as the sole RenderedFile of a RenderNodeOutput.
// The transform is identical across the per-language render fixtures, so
// the same RenderNodeInput yields equivalent output whichever language
// rendered it. The host verifies end-to-end that:
//
//   - The plugin loaded successfully via the Aggrega Wasm loader.
//   - The input decoded to a RenderNodeInput from the render socket's
//     canonical CBOR wire.
//   - The render transform ran against the node projection.
//   - The output encodes to a RenderNodeOutput whose file content reaches the
//     host verbatim: the render socket carries file content as a CBOR byte
//     string, so the guest sets the raw bytes and the host reads them back
//     unchanged.
//   - The facts the fixture reports about what it emitted reach the host in
//     the order listed here, each value byte for byte as written here.
//
// The wire types are re-exported from the PDK's aggrega package as aliases
// of the generated ABI codec types, so a value built here is the same
// value the codec encodes and the wire shape stays single-sourced from
// the schema.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o render-node-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// outputPath is the fixed on-disk path the fixture emits its single file
// at. Constant across the per-language render fixtures so their outputs
// compare equal.
const outputPath = "parity.txt"

// contentPrefix is the fixed prefix prepended to the node id to form the
// emitted file's bytes. Constant across the per-language render fixtures
// so identical input produces identical content.
const contentPrefix = "rendered-node:"

// outputFormat is the format the fixture declares its emitted file was
// rendered in, so the host can route the file to a matching parser. A
// provider that declares no format leaves the field unset and the host
// infers the format from the file extension.
const outputFormat = "text"

// descriptor is the static describe() self-report: a provider plugin
// whose render_node export the host validates before dispatch. The host reads
// this once at load through the shared describe() gate.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:   aggrega.APIVersion,
		PluginType:   "provider",
		Description:  "Render one IR node to a single output file whose bytes are the node id under a fixed prefix.",
		Capabilities: []string{"parity"},
		Exports: []aggrega.PluginExport{
			{Name: "render_node", InputPattern: "b", Stability: "experimental"},
		},
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

// echoedNode is the one entry of the echoed_node fact. Its fields are declared
// prefix first, and encoding/json writes a struct's fields in declaration
// order, so the entry's key order is this guest's own choice — a map here would
// have its keys sorted instead.
type echoedNode struct {
	Prefix string `json:"prefix"`
	ID     string `json:"id"`
}

// renderFacts returns the facts this fixture reports about what it emitted. A
// fact is a name and a list of entries, and both are this provider's own
// vocabulary: the host carries them in the order listed, refuses a value that is
// not a list, and learns nothing else about either. Every value is a list even
// with one entry, because the host joins the lists one name gets from every
// render of a node.
func renderFacts(nodeID string) ([]aggrega.RenderFact, error) {
	paths, err := json.Marshal([]string{outputPath})
	if err != nil {
		return nil, err
	}
	node, err := json.Marshal([]echoedNode{{Prefix: contentPrefix, ID: nodeID}})
	if err != nil {
		return nil, err
	}
	return []aggrega.RenderFact{
		{Name: "emitted_paths", Value: paths},
		{Name: "echoed_node", Value: node},
	}, nil
}

//go:wasmexport render_node
func render_node() int32 {
	var in aggrega.RenderNodeInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(fmt.Errorf("render-node-go: decode RenderNodeInput: %w", err))
		return 1
	}

	// The file content is a fixed function of the node id. Content is a
	// byte slice; the codec writes it as a CBOR byte string, and the host
	// reads it verbatim.
	content := []byte(contentPrefix + in.Node.ID)

	facts, err := renderFacts(in.Node.ID)
	if err != nil {
		pdk.SetError(fmt.Errorf("render-node-go: compose render facts: %w", err))
		return 1
	}

	out := aggrega.RenderNodeOutput{
		Files: []aggrega.RenderedFile{{
			Path:    outputPath,
			Content: content,
			Format:  outputFormat,
		}},
		RenderFacts: facts,
	}
	outputBytes, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("render-node-go: encode RenderNodeOutput: %w", err))
		return 1
	}

	pdk.Output(outputBytes)
	return 0
}

func main() {}
