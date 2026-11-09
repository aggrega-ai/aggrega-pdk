// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package aggrega

import abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"

// APIVersion is the Aggrega Wasm plugin ABI version this PDK targets,
// re-exported from the portable aggrega-abi module so a plugin author writes
// aggrega.APIVersion while the value itself stays single-sourced in the
// descriptor schema that declares it. The constant is the v1beta1 contract
// floor; cross-language PDKs (Go, Rust, TypeScript) export the same value
// because all three are generated from that one declaration.
const APIVersion = abi.AbiVersion

// Version and Commit are the plugin's build-provenance stamp. They are
// empty by default and are meant to be injected at build time via the Go
// linker, targeting these package-qualified symbols:
//
//	go build -buildmode=c-shared -o plugin.wasm \
//	  -ldflags "-X github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega.Version=1.2.3 \
//	            -X github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega.Commit=abc1234" \
//	  ./
//
// [MarshalDescriptor] folds each set value into the descriptor's metadata. A
// plain build (no -ldflags) leaves both empty, so an unstamped plugin's
// descriptor is byte-identical to a stamped one's minus the provenance fields.
var (
	Version string
	Commit  string
)
