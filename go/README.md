# aggrega-pdk-go

Go PDK for authoring Aggrega Wasm plugins.

Import paths: `github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega` (the socket payload
types and the host-function bindings every plugin uses) and
`github.com/aggrega-ai/aggrega-pdk-go/pkg/importer` (the library an importer plugin is
built on — see [Writing an importer](#writing-an-importer)).

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

### Writing an importer

An importer turns a foreign flow definition into draft Aggrega entities.
`pkg/importer` holds everything about that which does not depend on the source
format, so an importer plugin built on it carries only what is particular to its
one format.

What you write:

- the `describe()` descriptor: plugin type `importer`, the `import:<format>`
  capability, and an `import` export declaring the `aggrega_parse_source` host
  function;
- an `importer.Format`: the format's name, the syntactic format the host parses a
  source of it as, its mapping arm (`map[string]importer.Mapping`, from the node
  types the format names to the components they map to), and a walker lifting the
  `importer.Node` and `importer.Edge` values out of the canonical tree the host
  returns;
- the `import` export: decode the `aggrega.ImportInput` from its input with
  `DecodeCBOR`, hand it to `importer.Import` with your `Format` and a parse
  callback that calls `aggrega.ParseSource`, and return the `aggrega.ImportOutput`
  it produces encoded with `EncodeCBOR`.

The importer socket carries canonical CBOR in both directions: the input arrives
in it and the output goes back in it. An importer that cannot produce a result
says why in the output's `Error` field, and the host reports a non-empty `Error`
as that importer's failure. `settings` arrives as the bytes the attachment
authored, which `pkg/importer` reads as JSON.

Besides the source and the attachment's `settings`, the `aggrega.ImportInput` an
importer decodes carries `Catalog`, a view of the Components the workspace's own
catalog holds: an `aggrega.CatalogProjection` whose `Components` are sorted by
name, one entry per Component name at the version a reference naming it without
a version resolves to. Each is an `aggrega.CatalogComponent` carrying the
Component's name and version, its labels, the key names its values and provides
declare (each list sorted), and its sockets as `aggrega.CatalogSocket` entries
(name and label selector) in the order the Component declares them, the sockets a
Component inherits through `extends` first. It carries names and labels only —
never a value, a default or a secret — and it is nil when the catalog holds no
Component.

`pkg/importer` scores against that view and against nothing else: its candidates
are the workspace's own catalog, an enterprise catalog's Components exactly as
the standard library's, and the package carries no candidate of its own. Of each
Component the scorer reads the name, the category its
`component.aggrega.ai/category` label carries, its labels, and the key names its
values declare; provides keys and sockets are not weighed. A nil `Catalog` means
no candidate, so every node is drafted and none is referenced.

What the package gives you: the confidence scorer that weighs a node against the
catalog view, the decision to reference a component or draft a new one, the
drafted Component and Aggregate bodies, the `human` and `llm` renderers, and the
importer's settings — `target_namespace`,
`reuse_mode`, `reuse_threshold`, `emit_hints`, `render_mode` and
`mapping_overrides` — with their defaults. `NodeList`, `EdgeList` and
`ObjectEntries` read the shapes a walker commonly meets in a canonical tree. The
package builds on the host toolchain as well as for `GOOS=wasip1`, so an importer is
tested on the host by handing `Import` a parse function of the test's own. Its
package documentation carries the scoring signals, the tiers and the settings in
full.

The compiler's own Langflow and Flowise importers are built this way, one per
format, and they are the worked example to read: their sources are in the
compiler's repository (`github.com/aggrega-ai/aggrega-cli`), under
`plugins/langflow-importer/` and `plugins/flowise-importer/`. Each holds its
descriptor and export wiring (`main.go`), its walker (`walk.go`), and its format's
mapping arm and syntactic format (`format.go`), and takes everything else from
`pkg/importer`.

[`examples/import/`](examples/import) is this kit's teaching importer: a minimal
scaffold that declares a recognition signature and exercises both directions of
the import codec, answering with one drafted entity and one scaffold file rather
than a real mapping. It builds with its own `./build.sh`, which writes
`import-go.wasm` into `$AGGREGA_PDK_WASM_OUT` when that is set, and stamps the
version the guest reports through `describe()` from `AGGREGA_PLUGIN_VERSION`
(default `1.0.0`). The commit is left unstamped, so two builds of one tree are
byte-identical.

### Values an input marks sensitive

A plugin never quotes, in a refusal or a warning it returns, a value its input
marks sensitive, by its key or as part of a payload marked as a whole: the host
carries a plugin's refusal and warning text to the reader unchanged, so the
plugin is the only party able to keep the value out. That includes text a library
writes for the plugin — a template engine's, a schema validator's or a parser's
failure over a marked value. The marking arrives on the typed payloads: on a
provider's render input (`aggrega.RenderNodeInput`), `Node.SensitiveRenderedKeys`
by key; `Node.SensitiveValues`, `WorkloadProjection.SensitiveWorkload`, each
`ArtifactsProjection.Entries` element's `SensitiveContent` and each `Omits`
element's `Sensitive` for a whole payload whose producing template read a
declared value; and on each `Node.Features` entry
read as an `aggrega.IRFeatureEntry`, `SensitivePayloadLeaves`, `SensitiveInject`
and `Sensitive`; on the governance socket's `ir-verdict` input
(`aggrega.PolicyInput`), the same attachment fields and each node's
`SensitivePayloadLeaves` in the graph. The plugin contract in the Aggrega
documentation lists where each socket marks a key or a payload, and what an
absent marking does and does not say.

### The provider render socket

A provider guest's exports — `render_node`, and whichever optional exports its
`describe()` declares — read their input and write their output in canonical
CBOR. Decode the input with the input type's `DecodeCBOR` (`aggrega.RenderNodeInput`
for `render_node`) and return the output encoded with `EncodeCBOR`; a rendered
file's `Content` is the byte slice to write, which the encoder carries as raw
bytes. [`examples/render/`](examples/render) and [`examples/link/`](examples/link)
are providers built this way.

### The pipeline-step socket

A pipeline step's `step` export reads its input and writes its output in
canonical CBOR. Decode the input with `abi.FRPStepInput`'s `DecodeCBOR` and return
the `abi.FRPStepOutput` encoded with `EncodeCBOR`; the step types come from the
ABI package (`github.com/aggrega-ai/aggrega-abi-go/pkg/abi`) directly. The step's
`with` input, its outputs and its two document sets are order-keeping maps, so the
outputs a step sets reach `.steps.<id>.outputs` in the order it set them.
[`examples/frp-step/`](examples/frp-step) and the producer and injector examples
beside it are steps built this way.

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
`Omitted` empty. An entry whose `Sensitive` is set had at least one of its fields
produced from a value an author declared sensitive: match and count it like any
other, and keep every one of its fields out of a refusal, a warning or a log line,
as with every other value the input marks.

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
│   ├── source_wire.go, parse_source_wire.go, fetch_wire.go (host-callback wire helpers)
│   ├── host_funcs.go (wasip1-build-tagged host function bindings)
│   ├── host_funcs_stub.go (host-build stubs for non-wasm targets)
│   ├── host_error.go (host-call error handling)
│   └── aggrega_test.go (round-trip tests against the schemas)
├── pkg/importer/     (the importer library: scoring, decisions, drafts,
│                     rendering, settings, and the tree readers walkers use)
└── examples/         (one directory per worked plugin — a mutator, a
                      policy, a governance adapter skeleton, an FRP
                      step, a CLI plugin, a minimal importer, and
                      others)
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
