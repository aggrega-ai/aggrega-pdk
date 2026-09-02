# aggrega-pdk-go

Go PDK for authoring Aggrega Wasm plugins.

Import path: `github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega`.

The canonical reference for the plugin author's contract — what a plugin is,
the `describe()` self-report, exports and capability claims, runtime limits and
host functions, the `Plugin` entity, referencing a plugin, and the
publish → pull → build lifecycle — is the Wasm plugin developer guide in the
Aggrega documentation at [aggrega.ai](https://aggrega.ai). This README is the
Go-specific quickstart; the guide is the single home for the concepts.

## Authoring a plugin

A plugin is a small Go module that imports `pkg/aggrega` for the typed
socket payloads and the Aggrega host-function bindings, plus
`github.com/extism/go-pdk` for the input/output byte plumbing.

Build the plugin to a `.wasm` artifact with the standard Go toolchain
targeting `GOOS=wasip1`:

```sh
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o <plugin>.wasm ./
```

The result is a WASI reactor. The Aggrega Wasm loader runs it under a
deterministic, capability-denied WASI configuration — the
ahead-of-time-purity firewall the developer guide describes in full.

See `examples/mutator/` and `examples/policy/` for working
plugin sources.

### Starting from a skeleton

[`examples/governance-adapter-skeleton/`](examples/governance-adapter-skeleton)
is a copy-and-fill starting point for a governance plugin: one guest attachable
at every governance role, with no policy engine compiled in and one clearly
marked seam where an author drops their own evaluator. It is a skeleton rather
than a framework — the role wiring, the generated wire types and the pointer
escaping are done; the engine is not. Its README carries the role table, the four
things an adapter maps, and how to check the guest runs before attaching it.

## Build provenance

A plugin can stamp its version and commit into its `describe()`
descriptor. Inject the values at build time with the Go linker,
targeting the PDK's package-qualified symbols:

```sh
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm \
  -ldflags "-X github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega.Version=1.2.3 \
            -X github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega.Commit=abc1234" \
  ./
```

Build the descriptor with typed values and marshal it through
`aggrega.MarshalDescriptor`; it folds the stamped `Version` and `Commit`
into the descriptor's `metadata` object:

```go
out, err := aggrega.MarshalDescriptor(aggrega.PluginDescriptor{
    AbiVersion: aggrega.APIVersion,
    PluginType: "cli",
    Exports:    []aggrega.PluginExport{{Name: "cli_run"}},
    Metadata:   &aggrega.PluginDescriptorMeta{Author: "me"},
})
```

A plain build (no `-ldflags`) leaves the provenance empty and
`MarshalDescriptor` omits the `version` / `commit` members — the unstamped
descriptor is byte-identical to the stamped one minus those members.

## Layout

```text
go/
├── README.md         (this file)
├── go.mod            (module github.com/aggrega-ai/aggrega-pdk-go)
├── pkg/aggrega/
│   ├── doc.go        (package doc)
│   ├── version.go    (APIVersion constant)
│   ├── abi_reexport.go (socket payload types, aliased from aggrega-abi)
│   ├── describe.go   (descriptor helpers for the describe export)
│   ├── source_wire.go, parse_source_wire.go (source-wire helpers)
│   ├── host_funcs.go (wasip1-build-tagged host function bindings)
│   ├── host_funcs_stub.go (host-build stubs for non-wasm targets)
│   ├── host_error.go (host-call error handling)
│   └── aggrega_test.go (round-trip tests against the schemas)
└── examples/         (one directory per worked plugin — a mutator, a
                      policy, a governance adapter skeleton, an FRP
                      step, a CLI plugin, and others)
```

`types.go` carries no types: the socket payload types are generated
into the shared `aggrega-abi` module and aliased into this package by
`abi_reexport.go`, so `aggrega.<Type>` is the same type the codec
marshals. See the [kit overview](../README.md) for the generation and
the gates that hold it.

## Standalone reuse

The Go PDK's tests reference the JSON Schemas at relative path
`../../../aggrega-schemas/json/` (relative to `pkg/aggrega/`).
This assumes the umbrella workspace layout. Standalone consumption
(after publication to `pkg.go.dev`) bundles the schemas in the
release artifact; the file-relative reference becomes
publication-bundled. Until publication, only the umbrella checkout
runs the round-trip tests end-to-end.

When the schemas are not reachable those tests **fail** with a clear
diagnostic, because a run that could not read them has verified
nothing. Where they are genuinely absent — a standalone checkout of
this kit, or a packaging test — opt out with the same switch the Rust
and TypeScript kits use:

```sh
AGGREGA_SKIP_SCHEMA_TESTS=1 go test ./...
```

Each affected test is then reported as skipped, with the reason
stated. The switch keys on the schemas being unreachable: with them in
place it changes nothing, so it cannot quietly turn a real run into a
skipped one.
