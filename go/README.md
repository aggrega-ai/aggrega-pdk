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

### Values an input marks sensitive

A plugin never quotes, in a refusal or a warning it returns, a value its input
marks sensitive, by its key or as part of a payload marked as a whole: the host
carries a plugin's refusal and warning text to the reader unchanged, so the
plugin is the only party able to keep the value out. That includes text a library
writes for the plugin — a template engine's, a schema validator's or a parser's
failure over a marked value. The marking arrives on the typed payloads: on a
provider's render input (`aggrega.RenderNodeInput`), `Node.SensitiveRenderedKeys`
by key; `Node.SensitiveValues`, `WorkloadProjection.SensitiveWorkload` and each
`ArtifactsProjection.Entries` element's `SensitiveContent` for a whole payload
whose producing template read a declared value; and on each `Node.Features` entry
read as an `aggrega.IRFeatureEntry`, `SensitivePayloadLeaves`, `SensitiveInject`
and `Sensitive`; on the governance socket's `ir-verdict` input
(`aggrega.PolicyInput`), the same attachment fields and each node's
`SensitivePayloadLeaves` in the graph. The plugin contract in the Aggrega
documentation lists where each socket marks a key or a payload, and what an
absent marking does and does not say.

### Reporting facts about what a provider emitted

A provider's `render_node` may return `RenderFacts` beside its files: an ordered
list of `aggrega.RenderFact`, each a `Name` and a `Value` holding the JSON your
guest writes. Both are your provider's own vocabulary, which you document for the
authors who read them. **Report every fact's `Value` as a JSON array, even when it
holds one entry:** one name can be reported by more than one render of a
component — its own render calls and the renders of the Features attached to
it — and the host joins those lists into one under the name, so it refuses a
fact whose value is anything but a list and halts the build. That is all the host
learns of a value; it keys facts by name, hands each list to the templates that
read it, and writes the same files whether or not facts accompany them. A `Value`
is `json.RawMessage`, so marshal a slice of structs rather than of maps when the
order of an entry's keys matters: `encoding/json` writes a struct's fields in
declaration order and a map's keys sorted. `examples/render/` reports two facts.

### Leaving rendered units out

A source render may carry `Omits` beside its `SourceListing`: an ordered list of
`aggrega.OmitEntry`, each a `Kind`, a `Name` and an optional `Namespace`, already
resolved to plain strings. What an entry selects in your target's format is your
provider's own contract, which you document for the authors who write the
entries. A provider that honours them declares `Omits: true` on its
`ProviderContract`, matches every entry against what it rendered, leaves each
matched unit out of everything it returns — its `Files`, its `DerivedKeys` and its
`RenderFacts` — and returns `Omitted`: one count per entry, in the order the
entries arrived, each the number of units that entry matched whether or not
another entry matched them too. A zero is a complete answer. A render handed no
entries is rendered exactly as it would be without the field, and leaves
`Omitted` empty.

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
