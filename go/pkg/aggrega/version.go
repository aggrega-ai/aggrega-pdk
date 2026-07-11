// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package aggrega

import "strconv"

// APIVersion is the Aggrega Wasm plugin ABI version this PDK targets.
// The constant is the v1alpha1 contract floor; cross-language PDKs
// (Go, Rust, TypeScript) export the same value.
const APIVersion = "v1alpha1"

// Version and Commit are the plugin's build-provenance stamp. They are
// empty by default and are meant to be injected at build time via the Go
// linker, targeting these package-qualified symbols:
//
//	go build -buildmode=c-shared -o plugin.wasm \
//	  -ldflags "-X github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega.Version=1.2.3 \
//	            -X github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega.Commit=abc1234" \
//	  ./
//
// A plain build (no -ldflags) leaves both empty, and [ProvenanceMetadata]
// then contributes nothing, so an unstamped plugin's descriptor is
// byte-identical to a stamped one's minus the provenance fields.
var (
	Version string
	Commit  string
)

// ProvenanceMetadata returns the optional build-provenance fragment a
// plugin appends inside its describe() descriptor's "metadata" object.
// Each stamped field contributes a leading-comma JSON member — for
// example `,"version":"1.2.3","commit":"abc1234"` — so the fragment
// drops straight in after the fixed metadata members:
//
//	head := `{...,"metadata":{"author":"me"`
//	descriptor := head + aggrega.ProvenanceMetadata() + `}}`
//
// When neither [Version] nor [Commit] is stamped the fragment is empty,
// keeping the unstamped descriptor byte-identical to the stamped one
// minus the provenance members.
func ProvenanceMetadata() string {
	meta := ""
	if Version != "" {
		meta += `,"version":` + strconv.Quote(Version)
	}
	if Commit != "" {
		meta += `,"commit":` + strconv.Quote(Commit)
	}
	return meta
}
