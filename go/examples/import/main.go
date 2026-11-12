// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Reference importer plugin for the Aggrega Wasm runtime, authored with the Go
// PDK and built with the standard Go toolchain for GOOS=wasip1.
//
// An importer turns a foreign infrastructure description into Aggrega entities,
// so a consumer can adopt existing infrastructure without hand-authoring the
// entities from scratch. This example is a minimal scaffold rather than a real
// importer: it self-describes as an importer plugin advertising the import:demo
// format, and its import export echoes the input it receives back as a single
// drafted entity and a matching scaffold file. The point is to exercise both
// directions of the generated import codec — the export decodes an ImportInput
// and encodes an ImportOutput — not to implement a real mapping.
//
// A real importer weighs each node of a parsed source tree against the catalog's
// entities and records the decision behind what it drafts for the node. This
// scaffold skips the mapping and simply echoes the request into one emitted
// entity plus one scaffold file, so the example exercises the codec without a
// real source-to-entity mapping.
//
// It self-describes via describe() as an importer plugin whose capabilities
// list the import:demo format and whose single export is import. The host reads
// describe() once at load, validates the descriptor, then dispatches import
// with an ImportInput as input.
//
// The import payload is decoded and the result encoded through the generated ABI
// codec (github.com/aggrega-ai/aggrega-abi-go/pkg/abi, re-exported by this PDK)
// rather than hand-declared wire types, so the wire shape stays single-sourced
// from the schema. The importer socket carries canonical CBOR in both directions:
// the input arrives as canonical CBOR and the output is returned the same way.
//
// Build: ./build.sh, which stamps the version the guest reports through
// describe() (AGGREGA_PLUGIN_VERSION, default 1.0.0) into
// GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -ldflags "-X github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega.Version=<version>" -o import-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// descriptor is the static describe() self-report: an importer plugin
// whose import export handles the import:demo format. The host reads this once
// at load and validates it before invoking import.
//
// It also declares how a demo source is recognised when an import names no
// format: a YAML document whose top-level kind is the string DemoSource. The
// signature shows the shape every importer declares — the syntax the source is
// parsed in, and the paths its parsed tree must carry — and the host matches it
// without knowing what the demo format is.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:   aggrega.APIVersion,
		PluginType:   "importer",
		Description:  "Echo a source description back as one drafted entity and a scaffold file — a minimal importer scaffold.",
		Capabilities: []string{"import:demo"},
		Exports: []aggrega.PluginExport{{
			Name: "import",
		}},
		Importer: &aggrega.ImporterContract{Detect: []aggrega.DetectSignature{{
			Syntax: "yaml",
			Match:  []aggrega.PathMatch{{Path: "kind", Is: "string", Equals: "DemoSource"}},
		}}},
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

// importArtifact is the import export. The Go function is named differently
// from the export because "import" is a Go keyword; the //go:wasmexport
// directive carries the export name the host dispatches.
//
//go:wasmexport import
func importArtifact() int32 {
	// Decode the ImportInput from the canonical CBOR the socket carries, through
	// the generated codec's DecodeCBOR — no CBOR library, no reflective
	// struct-walk.
	var in aggrega.ImportInput
	if err := in.DecodeCBOR(pdk.Input()); err != nil {
		pdk.SetError(fmt.Errorf("import-go: decode ImportInput: %w", err))
		return 1
	}

	// Additively interpret two optional settings from the opaque options
	// document. When Settings is absent, name/mode keep their defaults so the
	// output is byte-identical to a settings-free run: target_namespace prefixes
	// the emitted entity name, and reuse_mode "auto-apply-above-threshold" emits
	// the entity as a reference rather than a draft. A malformed options document
	// leaves the defaults in place. This is a minimal demonstration of the
	// settings-threading path, not a real reuse scorer.
	var opts struct {
		TargetNamespace string `json:"target_namespace"`
		ReuseMode       string `json:"reuse_mode"`
	}
	if len(in.Settings) > 0 {
		_ = json.Unmarshal(in.Settings, &opts)
	}

	name := in.Format
	if opts.TargetNamespace != "" {
		name = opts.TargetNamespace + "-" + in.Format
	}
	mode := "draft"
	if opts.ReuseMode == "auto-apply-above-threshold" {
		mode = "reference"
	}

	// A real importer would read the parsed source and weigh each node against
	// the catalog's entities, recording the decision behind each. This
	// scaffold echoes the input as one drafted entity plus a matching scaffold
	// file so the example exercises both codec directions without a mapping.
	body := "kind: Component\nmetadata:\n  name: " + name + "\n"
	out := aggrega.ImportOutput{
		Entities: []aggrega.EmittedEntity{{
			Kind: "Component",
			Name: name,
			Mode: mode,
			Body: body,
		}},
		Files: []aggrega.ScaffoldFile{{
			Path:    name + ".yaml",
			Content: body,
		}},
	}

	// Reflect each named source back as an info diagnostic so a consumer can
	// confirm the host threaded with.sources through to the guest. Absent
	// sources add no diagnostics, keeping a single-file import byte-identical.
	for _, s := range in.Sources {
		out.Diagnostics = append(out.Diagnostics, aggrega.Diagnostic{
			Severity: "info",
			Message:  "received source: " + s.Name,
		})
	}

	// Encode the output in the canonical CBOR the socket carries, through the
	// generated codec's EncodeCBOR — no CBOR library, no reflective struct-walk.
	raw, err := out.EncodeCBOR()
	if err != nil {
		pdk.SetError(fmt.Errorf("import-go: encode ImportOutput: %w", err))
		return 1
	}
	pdk.Output(raw)
	return 0
}

func main() {}
