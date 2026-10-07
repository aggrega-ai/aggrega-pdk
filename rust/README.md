# aggrega-pdk-rust

Rust PDK for authoring Aggrega Wasm plugins.

Crate name: `aggrega-pdk-rust`.

The canonical reference for the plugin author's contract — what a plugin is,
the `describe()` self-report, exports and capability claims, runtime limits and
host functions, the `Plugin` entity, referencing a plugin, and the
publish → pull → build lifecycle — is the Wasm plugin developer guide in the
Aggrega documentation at [aggrega.ai](https://aggrega.ai). This README is the
Rust-specific quickstart; the guide is the single home for the concepts.

## Authoring a plugin

A plugin is a small Cargo crate that depends on `aggrega-pdk-rust`
for the typed socket payloads and the Aggrega host-function bindings,
plus `extism-pdk` for the plugin-function macro.

Build the plugin to a `.wasm` artifact with Cargo's
`wasm32-unknown-unknown` target:

```sh
cargo build --release --target wasm32-unknown-unknown
```

The artifact lands at
`target/wasm32-unknown-unknown/release/<crate>.wasm`. The crate's own
`build.sh` (where one ships) copies it into the umbrella's testdata
directory under the `<socket>-<lang>.wasm` naming convention the
host's integration tests consume.

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

### Two roads for a policy engine

Once the seam has an engine behind it, there are two ways to get that engine into
a guest, and the choice has consequences an author should make deliberately.

The **low-friction road** embeds a Wasm-native evaluator in the guest and reads
the policy from the attachment's files, so an operator authors the policy and
the guest carries no compiled policy module.
[`examples/policy-regorus/`](examples/policy-regorus) is that shape.

The **full-fidelity road** compiles the policy ahead of time with the policy
engine's own toolchain into a WebAssembly module, commits that module, and drives
it inside the guest with an embedded WebAssembly interpreter — so the guest gets
the reference engine's complete builtin surface and its exact semantics.
[`examples/policy-opa-wasm/`](examples/policy-opa-wasm) is that shape, and its
README documents how the guest instantiates the compiled module and satisfies its
imports in-guest.

It is the escape hatch rather than the default, and the trade is worth stating
before an author takes it: the full-fidelity road buys builtin completeness at
the cost of artifact size — the guest carries both an interpreter and the
compiled module — plus an ahead-of-time build step whose output is committed, and
a policy fixed at build time rather than supplied by the operator. Reach for it
when a policy needs a builtin the Wasm-native evaluator does not implement, or
when a decision has to match the reference engine exactly.

### Values an input marks sensitive

A plugin never quotes, in a refusal or a warning it returns, a value its input
marks sensitive, by its key or as part of a payload marked as a whole: the host
carries a plugin's refusal and warning text to the reader unchanged, so the
plugin is the only party able to keep the value out. That includes text a library
writes for the plugin — a template engine's, a schema validator's or a parser's
failure over a marked value. The typed payloads carry the marking under its wire
names: on a provider's render input, `node.sensitive_rendered_keys` by key;
`node.sensitive_values`, `workload_projection.sensitive_workload` and each
`artifacts_projection.entries` element's `sensitive_content` for a whole payload
whose producing template read a declared value; and on each `node.features` entry
read as the IR's feature entry, `sensitive_payload_leaves`, `sensitive_inject` and
`sensitive`; on the governance socket's `ir-verdict` input, the same attachment
fields and each node's `sensitive_payload_leaves` in the graph. The plugin
contract in the Aggrega documentation lists where each socket marks a key or a
payload, and what an absent marking does and does not say.

### Reporting facts about what a provider emitted

A provider's `render_node` may return `render_facts` beside its files: an
ordered `Vec<RenderFact>`, each a `name` and a `value` holding the JSON your
guest writes as a `Box<RawValue>`. Both are your provider's own vocabulary,
which you document for the authors who read them. **Report every fact's `value`
as a JSON array, even when it holds one entry:** one name can be reported by more
than one render of a component — its own render calls and the renders of the
Features attached to it — and the host joins those lists into one under the
name, so it refuses a fact whose value is anything but a list and halts the
build. That is all the host learns of a value; it keys facts by name, hands each
list to the templates that read it, and writes the same files whether or not
facts accompany them. Build an entry from a struct or from text rather than from
a `serde_json::Value` object when the order of its keys matters: serde writes a
struct's fields in declaration order, while `serde_json`'s default map sorts its
keys. `examples/render-node/` reports two facts.

## Build provenance

A plugin can stamp its version and commit into its `describe()`
descriptor. Inject the values at build time through the `AGGREGA_VERSION`
and `AGGREGA_COMMIT` compile-time environment variables:

```sh
AGGREGA_VERSION=1.2.3 AGGREGA_COMMIT=abc1234 \
  cargo build --release --target wasm32-wasip1
```

Build the descriptor with typed values and encode it through the
`describe!` macro, which folds the stamped `AGGREGA_VERSION` /
`AGGREGA_COMMIT` into the descriptor's `metadata` object:

```rust
use aggrega_pdk_rust::{describe, PluginDescriptor, PluginExport, API_VERSION};
let wire = describe!(PluginDescriptor {
    abi_version: API_VERSION.to_string(),
    plugin_type: "cli".to_string(),
    exports: vec![PluginExport { name: "cli_run".to_string(), ..Default::default() }],
    ..Default::default()
});
```

`describe!` is a macro, not a function, on purpose. It reads the
variables via `option_env!`, which is evaluated where the macro expands —
the plugin's own crate — so the values track *your* build. A plain PDK
library function would instead capture the environment at the PDK's own
(cached) compile and report a stale value. A plain build (neither
variable set) leaves the provenance empty and the encoded descriptor
omits the `version` / `commit` members — byte-identical to the stamped
one minus those members. A guest that has no build-time stamp uses
`encode_descriptor(&descriptor)` directly.

## Layout

```text
rust/
├── README.md           (this file)
├── Cargo.toml          (workspace root; crate aggrega-pdk-rust + fixture members)
├── Cargo.lock          (workspace-wide lockfile shared by all members)
├── src/
│   ├── lib.rs          (module declarations + the aggrega_abi re-exports)
│   ├── version.rs      (API_VERSION constant)
│   ├── describe.rs     (descriptor helpers for the describe export)
│   └── host_funcs.rs   (#[host_fn] bindings + safe wrappers)
├── tests/
│   └── round_trip.rs   (round-trip tests against the schemas)
└── examples/           (one directory per worked plugin — a mutator, a
                        policy, a governance adapter skeleton, FRP
                        steps, a CLI plugin, and others)
```

`types.rs` carries no types: the socket payload types are generated
into the shared `aggrega-abi` crate and re-exported from `lib.rs` with
`pub use`, so the spelling a plugin author writes is the type the codec
serializes. See the [kit overview](../README.md) for the generation and
the gates that hold it.

## Standalone reuse

The Rust PDK's tests reference the JSON Schemas at relative path
`../../aggrega-schemas/json/` (relative to this crate root).
This assumes the umbrella workspace layout. Standalone consumption
(after publication to `crates.io`) bundles the schemas in the release
artifact; the file-relative reference becomes publication-bundled.

When running `cargo test` from a checkout that does not have the
schemas alongside, the schema-dependent tests **fail** with a clear
diagnostic, because a run that could not read them has verified
nothing. Where they are genuinely absent — a standalone checkout of
this kit, or a packaging test — opt out with the same switch the Go
and TypeScript kits use:

```sh
AGGREGA_SKIP_SCHEMA_TESTS=1 cargo test
```

Each affected test then prints the reason it was skipped. Rust's
stable test framework has no dynamic-skip result, so a test that
decided at run time not to check anything is still counted as
passing; the printed line names the test and says plainly that it
verified nothing, which is what keeps that count from being read as a
verified pass. The switch keys on the schemas being unreachable: with
them in place it changes nothing, so it cannot quietly turn a real run
into a skipped one.

The umbrella's checkout always has the schemas in place, so neither
the refusal nor the skip fires there.

## Toolchain pin requirements

The Rust 1.85 floor (the umbrella's `setup-workspace.sh` minimum) is
held in place by transitive-dependency version pins inside
`Cargo.lock`. Specifically, `idna_adapter` must stay at the 1.1.x
line or earlier: the 1.2.x line requires Rust 1.86 (via
`icu_normalizer >= 2.2`).

Contributors editing `Cargo.toml` or running `cargo update --aggressive`
risk breaking the floor. Two mitigations are in place:

1. The `Cargo.lock` is committed, so plain `cargo update` (without
   `--aggressive` or `--precise` targeting these crates) respects
   the existing pins.
2. The PDK CI workflow at `.github/workflows/pdk.yml` runs a
   verification step that fails the build if `Cargo.lock` resolves
   `idna_adapter` beyond the 1.1.x line.

When the project's Rust floor moves to 1.86+, both the pins in
`Cargo.lock` and the verification step in the workflow drop together.
