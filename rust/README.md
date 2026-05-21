# aggrega-pdk-rust

Rust PDK for authoring Aggrega Wasm plugins.

Crate name: `aggrega-pdk-rust`.

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

See `examples/mutator/` and `examples/policy-validator/` for working
plugin sources.

## Layout

```text
rust/
├── README.md           (this file)
├── Cargo.toml          (workspace root; crate aggrega-pdk-rust + fixture members)
├── Cargo.lock          (workspace-wide lockfile shared by all members)
├── src/
│   ├── lib.rs          (module declarations + public re-exports)
│   ├── version.rs      (API_VERSION constant)
│   ├── types.rs        (socket payload types — derive(Serialize, Deserialize))
│   └── host_funcs.rs   (#[host_fn] bindings + safe wrappers)
├── tests/
│   └── round_trip.rs   (round-trip mirroring tests against the schemas)
└── examples/
    ├── mutator/        (fixture mutator plugin)
    └── policy-validator/ (fixture policy-validator plugin)
```

## Standalone reuse

The Rust PDK's tests reference the JSON Schemas at relative path
`../../aggrega-schemas/json-schemas/` (relative to this crate root).
This assumes the umbrella workspace layout. Standalone consumption
(after publication to `crates.io`) bundles the schemas in the release
artifact; the file-relative reference becomes publication-bundled.

When running `cargo test` from a checkout that does not have the
schemas alongside, the schema-dependent tests **fail** (Rust's stable
test framework has no dynamic-skip analog to Go's `t.Skipf`; an early
`return` would mark them PASSED and mask the setup gap). Skip them
explicitly via:

```sh
cargo test --workspace -- --skip round_trip
```

The umbrella's CI always has the schemas in place; the panic path
does not fire there.

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
