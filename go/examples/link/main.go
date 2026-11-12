// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Fixture provider plugin exercising the cross-boundary edge-linking
// exports, authored with the Go PDK and built with the standard Go
// toolchain for GOOS=wasip1.
//
// It advertises cross_boundary_linking in describe() and offers both link
// exports as the paired contract the host routes a cross-boundary uses
// edge through:
//
//   - link_publish runs on the consumed (target) endpoint. It derives a
//     deterministic pointer string from the edge, returns it as the
//     published handle the consumer resolves against, and returns one
//     structural patch that records the pointer on the consumed
//     endpoint's assembled file.
//   - link_resolve runs on the consuming (source) endpoint. It reads the
//     published handle and returns one structural patch that injects the
//     pointer into the consuming endpoint's assembled file.
//
// Each patch operation is authored as opaque JSON matching the shared
// pipeline patch-operation wire shape (scope/key/op/path/value); the host
// carries it verbatim and applies it through its codec. Output is a pure
// function of the input, so identical input yields byte-identical output.
//
// The mandatory render_node export is present and minimal — the link pass
// never invokes it, but a provider plugin must declare it.
//
// The wire types are re-exported from the PDK's aggrega package as aliases
// of the generated ABI codec types, so the wire shape stays single-sourced
// from the schema. The render socket carries canonical CBOR in both
// directions, so each export decodes its input and encodes its output through
// those types' binary codec.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o link-cross-boundary-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// linkFile is the assembled file each endpoint's dispatch group emits and
// this fixture wires. Both the consumed and consuming endpoints carry a
// file of this name in their own dispatch group; the two are distinct
// files in distinct groups.
const linkFile = "boundary.yaml"

// descriptor is the static describe() self-report: a provider plugin
// that declares the cross_boundary_linking capability and both link exports as
// a pair, alongside the mandatory render_node export. The host reads this once
// at load through the shared describe() gate.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:   aggrega.APIVersion,
		PluginType:   "provider",
		Description:  "Wire a cross-boundary uses edge by publishing a deterministic pointer and resolving it into each endpoint's assembled file.",
		Capabilities: []string{"link-cross-boundary"},
		Exports: []aggrega.PluginExport{
			{Name: "render_node", InputPattern: "b", Stability: "experimental"},
			{Name: "link_publish", InputPattern: "b", Stability: "experimental"},
			{Name: "link_resolve", InputPattern: "b", Stability: "experimental"},
		},
		Contract: &aggrega.ProviderContract{
			InputKinds:           aggrega.InputKinds{ArtifactsStandalone: true},
			BoundaryAssembly:     "none",
			CrossBoundaryLinking: true,
		},
		Metadata: &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
}

// derivedPointer builds the deterministic pointer string a consumed
// endpoint publishes for one link: the target node id and the socket alias
// the edge is bound under, joined by a fixed separator. The compiler pipes
// it as an opaque scalar; the runtime environment resolves it at deploy
// time.
func derivedPointer(targetNode, socket string) string {
	return targetNode + "--" + socket
}

// oneOp authors a single structural patch operation as opaque JSON
// matching the shared pipeline patch-operation wire shape and returns it as
// a json.RawMessage the host carries verbatim. It addresses the file's own
// root by the file's segmentation token (its emitted name). Field order is
// fixed so the emitted op is byte-identical across runs of identical input.
func oneOp(file, path, value string) (json.RawMessage, error) {
	op := struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}{
		Scope: "own",
		Key:   file,
		Op:    "add",
		Path:  path,
		Value: value,
	}
	return json.Marshal(op)
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

//go:wasmexport render_node
func render_node() int32 {
	// The link pass never routes render_node; a provider plugin must
	// still declare it. Emit nothing.
	out := aggrega.RenderNodeOutput{}
	outputBytes, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("link-cross-boundary-go: encode RenderNodeOutput: %w", err))
		return 1
	}
	pdk.Output(outputBytes)
	return 0
}

//go:wasmexport link_publish
func link_publish() int32 {
	var in aggrega.LinkPublishInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(fmt.Errorf("link-cross-boundary-go: decode LinkPublishInput: %w", err))
		return 1
	}
	pointer := derivedPointer(in.Edge.Target.Node, in.Edge.Socket)

	op, err := oneOp(linkFile, "/published", pointer)
	if err != nil {
		pdk.SetError(fmt.Errorf("link-cross-boundary-go: author publish op: %w", err))
		return 1
	}

	out := aggrega.LinkPublishOutput{
		Handle: map[string]string{"ref": pointer},
		Patches: []aggrega.LinkPatch{{
			File: linkFile,
			Ops:  []json.RawMessage{op},
		}},
	}
	outputBytes, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("link-cross-boundary-go: encode LinkPublishOutput: %w", err))
		return 1
	}
	pdk.Output(outputBytes)
	return 0
}

//go:wasmexport link_resolve
func link_resolve() int32 {
	var in aggrega.LinkResolveInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(fmt.Errorf("link-cross-boundary-go: decode LinkResolveInput: %w", err))
		return 1
	}
	// Inject the peer's published pointer verbatim into the consuming
	// endpoint's assembled file.
	pointer := in.Handle["ref"]

	op, err := oneOp(linkFile, "/resolved", pointer)
	if err != nil {
		pdk.SetError(fmt.Errorf("link-cross-boundary-go: author resolve op: %w", err))
		return 1
	}

	out := aggrega.LinkResolveOutput{
		Patches: []aggrega.LinkPatch{{
			File: linkFile,
			Ops:  []json.RawMessage{op},
		}},
	}
	outputBytes, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("link-cross-boundary-go: encode LinkResolveOutput: %w", err))
		return 1
	}
	pdk.Output(outputBytes)
	return 0
}

func main() {}
