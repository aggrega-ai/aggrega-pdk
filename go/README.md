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

A file imported on its own arrives as the request's `Bytes`, and its name — the
file's base name, with no directory — as its `Name`; a directory's files arrive
as `Sources`, each under its own name, with `Bytes` empty and the directory's
base name as `Name`. `importer.Sources`
hands the primary bytes on as a source with no name, and that is how
`pkg/importer` drafts them. To name that file in what an importer reports, pass
the source through `importer.Named`, as `importer.Import` does before its
`parse-failed` and `no-nodes` diagnostics, so an importer reporting with
`importer.ParseFailed` or `importer.NoNodes` itself names the file the same way.

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
no candidate, so no node is referenced, and every node the draft gives a
Component of its own is drafted.

What the package gives you: the confidence scorer that weighs a node against the
catalog view, the decision to reference a component or draft a new one, the
drafted Component and Aggregate bodies, the platform layout below, the `human` and
`llm` renderers, and the importer's settings — `target_namespace`,
`reuse_mode`, `reuse_threshold`, `emit_hints`, `render_mode`,
`mapping_overrides`, `layout`, `empty_credentials` and `attach_egress_allowlist` — with their defaults. `NodeList`, `EdgeList` and
`ObjectEntries` read the shapes a walker commonly meets in a canonical tree. The
package builds on the host toolchain as well as for `GOOS=wasip1`, so an importer is
tested on the host by handing `Import` a parse function of the test's own. Its
package documentation carries the scoring signals, the tiers and the settings in
full.

The drafted Aggregate is named `<target_namespace>/<name>`, its `<name>` named
after the source by `importer.DraftName`: the name the source gives itself, which
the format's optional `Format.Title` reads off each source's tree — a flow's, a
workflow's or an app's name, in the field the format declares it in — where every
source drafted gives the same one; else the name of the directory the import was
handed, whole, or the stem of the file it was handed; else `flow`. Each is
lowercased, with every run of characters other than ASCII letters and digits
written as one hyphen and the hyphens at either end dropped. An importer drafting an Aggregate or a
Release of its own names it through `importer.DraftName` too, handing it the names
its sources give themselves, so one source is named one way by every importer.

#### A format whose flows a platform runs

A format whose flows run on a platform as they are written — a Langflow flow on
Langflow, a Flowise chatflow on Flowise — sets `Format.Platform` to an
`importer.Platform`, and `Import` then drafts each source, by default, as one
instance of the platform's Component carrying the source, instead of every node as
a Component of its own. The `layout` setting chooses: `platform`, the default, or
`nodes`, the node-by-node draft a format with no `Platform` always gets; any other
value drafts as `platform`, with an `unknown-layout` warning. What a format supplies
there:

- **The platform and its two values.** `Component` names the catalog Component that
  runs a flow — its last segment is the instance's key — `DefinitionValue` the value
  carrying the flow, and `IdentifierValue` the value naming it on the platform. The
  source is carried as its own bytes, with only the values below written anew, every
  carriage return written as a line feed, every `{{` escaped for the Aggregate's
  template reading, and the whole written as a YAML literal block; a platform's sources
  are read as JSON to find those values, unless the format writes them itself, below.
- **The identifier.** `Identifier` reads the flow's own identifier off the source's
  tree. Where it gives none, or one holding a character other than a letter, a digit,
  `.`, `-`, `_` or `:`, the package derives a name-based UUID from the carried flow.
- **The credential locator.** `Credentials` returns an `importer.Credential` for every
  value in the source that can carry a credential, each addressed by a JSON Pointer
  from the source's root (`importer.Pointer` writes one) and tagged with its `Class`:
  `CredentialMarked` for what the format marks as a credential,
  `CredentialServerMember` for a member of an MCP server's configuration, and
  `CredentialUnmarked` for an input the format marks as none that can carry one, such
  as a request's headers. It returns every class every time and reads no setting: the
  package reads `empty_credentials` — `none`, the default, `marked` (the first two
  classes) or `all` — and writes empty the classes it names before the source is
  carried, each as `""` or as `Credential.Value`, for a field keeping its credentials
  inside text of its own, reported as `credential-emptied`. Every other value is
  carried as written and, where it holds something, reported as `credential-carried`
  with the setting value that writes it empty — by node and field, never by value. Any
  other setting value halts the import, which drafts nothing. A value `Wire` or
  `Reached` writes is theirs, neither emptied nor carried. A located value that is one
  variable reference and nothing else — the bare name `${NAME}` (a predefined variable
  such as `${workspaceFolder}` included), `${env:NAME}` or `${input:id}`, as
  `importer.IsVariableReference` reads one — holds no value: the package leaves it as
  written under every setting value and reports it nowhere, so the locator returns it
  like any other. A reference carrying text after an operator — `${NAME:-value}`,
  `${NAME-value}`, `${NAME:+value}`, `${NAME?message}`, `${NAME=value}` and the rest,
  with or without a colon — holds that text, which can be a key, and is located like
  any other value, as is any other text inside `${…}`. A locator writing
  `Credential.Value` keeps each part of the value that is one reference as written,
  since the package sees only the whole value. An
  importer that drafts values of its own rather than carrying a source calls
  `importer.CarryCredentials` with the values its own locator finds, for the same
  decision under the same codes; it reads references in its format's own grammar,
  `importer.IsVariableReference` where that is the MCP clients', and hands the
  package none of them.
- **A source that is not JSON.** `EmptyInSource` writes the values the setting writes
  empty in the source's own bytes, for a format whose sources are written in another
  syntax, as a Dify DSL is written in YAML. The package hands it only the values whose
  tree holds something to write, parses what it returns through the host's parser,
  and carries it only where that reads back as the source's tree with exactly those
  values written; otherwise the source is not carried, reported as
  `definition-not-carried`. A source the setting leaves as written is carried byte for
  byte, and `EmptyInSource` is never asked.
- **The selector matcher.** `Match` reports whether a set of labels satisfies a
  socket's selector. The package carries no selector grammar of its own, so a nil
  `Match` places no part in any socket; the compiler's importers hand it the
  compiler's own selector matching.
- **How a node reaches a part.** A node is a part where a mapping row maps it or its
  best candidate is at least a medium-tier match, and the part is wired to the first
  socket of the platform's catalog entry, in the order the entry lists its sockets,
  whose selector its Component's labels satisfy. `Reach` lets a node name a part
  rather than be one, as an MCP Tools node names the MCP server it calls: a node given
  a `Reach.Name` is a part whatever its scores, the nodes of one source giving one name
  in one socket share one part and one connection, and the connection takes that name
  as its alias wherever an alias can; `Reach.Refused` keeps the node in the flow,
  reported with why.
- **Which sockets refuse a node.** `Refuse` says why the flow cannot reach a part
  through one socket by way of one node, as Langflow reaches a tools connection only
  through an MCP Tools node naming it; the part passes that socket over for the next
  one its Component satisfies, or stays in the flow, reported.
- **Which sockets draw one connection.** `Single` lists them, each an
  `importer.SingleSocket` whose `Prefer` labels mark the part that holds the
  connection where several are placed there — the first part, where none carries them;
  every other stays in the flow, reported, and `Standing` says how the flow reaches its
  part as its node is written.
- **How the flow is rewired.** `Wire` is asked about each wired part, an
  `importer.Part` carrying the node, the socket, the part's key and its connection's
  alias, and answers with an `importer.Wiring`: the `Edits` written into the carried
  source so the flow reaches the part through its connection — a variable's name, an
  address emptied, a server renamed to its connection's alias — and `Reads` or
  `Literal`, saying how the flow reaches the part through that connection or other than
  through it, which the import reports as `part-reads-connection` or
  `part-address-in-flow`. A value a node would send in place of what its connection
  supplies, such as its own default headers, is written empty here, as wiring, and not
  by the credential locator. `Reached` is asked the same of each node the draft wires
  to no socket, with the parts it does wire: where a connection reaches the node as it
  is written — its client reading what the platform delivers from its environment — its
  `Edits` are written and its `Reads` reported as `part-reads-connection`.
  `importer.Rewrite` writes such edits into JSON text a format
  holds inside a value of its own, and `importer.VariableName` gives the name a
  platform delivering a connection under variables named for its alias gives that
  alias.
- **The model a node selects.** `Model` reads, off the source's tree, the name of the
  model a node selects. A part the draft references whose catalog entry declares a
  value named `model` is written with that name as `values.model`; where the node names
  none, a comment leaves it for the author and the import reports `model-not-named`.
- **The lines a target needs.** `TargetAttachments` are attachments of the platform's
  own instance that its Recipes on one target need declared in the Aggregate. The draft
  stays alike for every target and shows each, commented out inside the instance, for
  an author building for that target to uncomment.
- **What is left for the Release.** `LeftForRelease` names the platform's own inputs no
  source supplies, and each `Wiring.LeftForRelease` what of a part the Release
  supplies; the draft lists both in a comment above the platform's instance.
- **A platform that is a whole installation.** `Module`, an `importer.Module`, names
  the catalog Aggregate assembling the platform, `Component` among its parts, and the
  `Parameter` of it that takes the flow. Each source is then drafted in the module
  form: one entry of that Aggregate under `aggregates`, carrying the source in that
  parameter, with no identifier, no part drafted and nothing wired, since the module
  wires every connection of the platform itself. `PassedDown` lists the module's
  parameters no source supplies and the module gives no default, such as a key the
  installation signs with: the draft declares each once, as a required string with no
  default, and passes it down by name to every entry, for the Release to fulfill.
  `Configured` says what a node names that the platform configures for itself — a
  model and its provider, a tool, a knowledge base, a plugin — which the import
  reports in the node's decision and as `part-configured-in-platform`. The comment
  above the entry names each parameter passed down and `LeftForRelease`, and that the
  import, which sees the catalog's Components and none of its Aggregates, does not
  check that the catalog holds the module.

Every node the draft gives no Component stays inside the flow the platform carries,
and its decision chooses nothing and says so. Where the catalog view holds no
Component of the platform's name, the platform is drafted all the same, no part is
wired, and the import warns with `platform-not-in-catalog`. With the
`attach_egress_allowlist` setting `true`, every importer built on the package attaches
`core/egress-allowlist` to each platform instance it drafts, under the key
`egress-allowlist`, and sets none of the Feature's infrastructure: the comment above
the instance names `infra.control_plane_present` and `infra.admitted_destinations` for
the Release, and the import reports the attachment as `feature-attached`. A draft made
node by node attaches nothing, and warns with `feature-not-attached`. The package
documentation carries the layout in full.

The compiler's own Langflow and Flowise importers are built this way, one per
format, and they are the worked example to read: their sources are in the
compiler's repository (`github.com/aggrega-ai/aggrega-cli`), under
`plugins/langflow-importer/` and `plugins/flowise-importer/`. Each holds its
descriptor and export wiring (`main.go`), its walker and credential locator
(`walk.go`), its format's mapping arm, syntactic format and platform (`format.go`),
how its flow reaches each part (`wire.go`) and the model a node selects
(`model.go`), and takes everything else from `pkg/importer`.

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

### Accepting the files a Recipe contributes to the boundary

A Recipe may declare files under `spec.boundary.artifacts` that belong to the
shared output of the boundary it renders in rather than to one component's
output — a provider configuration your target reads once for the whole
boundary, say. The host evaluates those files, merges them across every
contributor by name, and writes each one once in the directory holding the
dispatch group's assembled output, beside what your `assemble_boundary` returns.
Your guest is handed none of them and reads none of them. Declare
`BoundaryArtifacts: true` on your `ProviderContract` only when your target
treats a file placed in that directory as part of the boundary's output; a
Recipe declaring the block for a provider that does not is refused before
anything renders. A file your assembly writes under the same name as one of
those files halts the build, so keep the names your assembly writes documented
for the authors who choose theirs.

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

## Compatibility

This kit follows semantic versioning from `0.9.0`. A plugin built
against a version keeps building against every later `0.9.x` release:
a release may add exported functions, types, constants and struct
fields to `pkg/aggrega` and `pkg/importer`, and never removes or changes
one. An added field is compatible for struct literals written with
field names, the form Go's own compatibility promise assumes. A
breaking change ships only as a new minor version below `1.0.0`
(`0.10.0`), or a new major version from `1.0.0`. The
[kit overview](../README.md#versioning-and-compatibility) states the
rule and the check that holds it.

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
This assumes the `aggrega-schemas` repository is checked out beside
this repository. Standalone consumption (after publication to
`pkg.go.dev`) bundles the schemas in the release artifact; the
file-relative reference becomes publication-bundled. Until
publication, only a checkout with `aggrega-schemas` beside it runs the
round-trip tests end-to-end.

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
