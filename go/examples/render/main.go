// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

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
//   - Input JSON deserialised to RenderNodeInput.
//   - The render transform ran against the node projection.
//   - Output JSON serialises to a RenderNodeOutput whose file content
//     round-trips through the render socket's base64 wire encoding. The
//     render socket carries file content as base64 on the JSON wire; the
//     guest sets the raw bytes and the codec base64-encodes them, and the
//     host decodes them verbatim.
//
// The wire types are re-exported from the PDK's aggrega package as aliases
// of the generated ABI codec types, so a value built here is the same
// value the codec marshals and the wire shape stays single-sourced from
// the schema.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o render-node-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// outputPath is the fixed on-disk path the fixture emits its single file
// at. Constant across the per-language render fixtures so their outputs
// compare equal.
const outputPath = "parity.txt"

// contentPrefix is the fixed prefix prepended to the node id to form the
// emitted file's bytes. Constant across the per-language render fixtures
// so identical input produces identical content.
const contentPrefix = "rendered-node:"

// pluginDescriptor is the static describe() self-report: a v1alpha1
// provider plugin whose render_node export the host validates before
// dispatch. The host reads this once at load through the shared describe()
// gate.
const pluginDescriptor = `{"abi_version":"v1alpha1","plugin_type":"provider","description":"Render one IR node to a single output file whose bytes are the node id under a fixed prefix.","capabilities":["parity"],"exports":[{"name":"render_node","input_pattern":"b","stability":"experimental"}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}`

//go:wasmexport describe
func describe() int32 {
	pdk.Output([]byte(pluginDescriptor))
	return 0
}

//go:wasmexport render_node
func render_node() int32 {
	var in aggrega.RenderNodeInput
	if err := json.Unmarshal(pdk.Input(), &in); err != nil {
		pdk.SetError(fmt.Errorf("render-node-go: unmarshal RenderNodeInput: %w", err))
		return 1
	}

	// The file content is a fixed function of the node id. Content is a
	// byte slice; the codec base64-encodes it on the JSON wire, and the
	// host decodes it verbatim.
	content := []byte(contentPrefix + in.Node.ID)

	out := aggrega.RenderNodeOutput{
		Files: []aggrega.RenderedFile{{
			Path:    outputPath,
			Content: content,
		}},
	}
	outputBytes, err := json.Marshal(out)
	if err != nil {
		pdk.SetError(fmt.Errorf("render-node-go: marshal RenderNodeOutput: %w", err))
		return 1
	}

	pdk.Output(outputBytes)
	return 0
}

func main() {}
