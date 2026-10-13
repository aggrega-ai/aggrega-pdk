// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Reference parser plugin for the Aggrega Wasm runtime, authored with the Go
// PDK and built with the standard Go toolchain for GOOS=wasip1.
//
// A parser turns a foreign source text into a canonical structured tree, so
// any consumer can read the tree without carrying its own language parser.
// This example is a minimal scaffold rather than a real language parser: it
// self-describes as a parser plugin advertising the parse:demo format, and its
// parse export echoes the input it receives back as a small structured tree.
// The point is to exercise both directions of the generated parse codec — the
// export decodes a ParseInput and encodes a ParseOutput — not to implement a
// grammar.
//
// It self-describes via describe() as a parser plugin whose capabilities list
// the parse:demo format and whose single export is parse. The host reads
// describe() once at load, validates the descriptor, then dispatches parse with
// a ParseInput as input.
//
// The parse payload is decoded and re-encoded through the generated ABI codec
// (github.com/aggrega-ai/aggrega-abi-go/pkg/abi, re-exported by this PDK) rather
// than hand-declared wire types, so the wire shape stays single-sourced from
// the schema. The parser socket carries canonical CBOR in both directions: the
// input is read with ParseInput.DecodeCBOR and the output written with
// ParseOutput.EncodeCBOR, so the source bytes arrive as a raw byte string in
// in.Bytes and the tree goes back as a byte string holding its JSON document.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o parse-go.wasm ./
package main

import (
	"encoding/json"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// descriptor is the static describe() self-report: a v1alpha1 parser plugin
// whose parse export handles the parse:demo format. The host reads this once at
// load and validates it before invoking parse.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:   aggrega.APIVersion,
		PluginType:   "parser",
		Description:  "Echo a source text back as a small structured tree — a minimal parser scaffold.",
		Capabilities: []string{"parse:demo"},
		Exports: []aggrega.PluginExport{{
			Name: "parse",
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

//go:wasmexport parse
func parse() int32 {
	// Decode the ParseInput from canonical CBOR through the generated codec's
	// DecodeCBOR — a direct walk into the typed value, no reflective struct-walk.
	var in aggrega.ParseInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(err)
		return 1
	}

	// A real parser would read in.Bytes as in.Format and build the canonical
	// tree. This scaffold echoes the input as a small object tree so the
	// example exercises both codec directions without a grammar.
	tree, err := json.Marshal(struct {
		Format string `json:"format"`
		Source string `json:"source"`
	}{
		Format: in.Format,
		Source: string(in.Bytes),
	})
	if err != nil {
		pdk.SetError(err)
		return 1
	}

	out := aggrega.ParseOutput{Tree: tree}

	// Encode the output to canonical CBOR through the generated codec's
	// EncodeCBOR — a manual field-ordered build, no reflective struct-walk.
	raw, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(raw)
	return 0
}

func main() {}
