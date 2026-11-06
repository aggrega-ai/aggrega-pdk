# aggrega-pdk

This repository holds the Plugin Development Kits for authoring
[Aggrega](https://aggrega.ai) Wasm plugins, one per supported source
language.

- **`go/`** — Go PDK; targets publication to `pkg.go.dev` as
  `github.com/aggrega-ai/aggrega-pdk-go`.
- **`rust/`** — Rust PDK; targets publication to `crates.io` as
  `aggrega-pdk-rust`.
- **`typescript/`** — TypeScript PDK; targets publication to npm as
  `@aggrega-ai/aggrega-pdk-typescript`.

Each PDK is a thin wrapper over the corresponding upstream
[Extism](https://extism.org) PDK (`extism-pdk-<language>`), adding
Aggrega-typed builders for the Wasm socket payloads and bindings for
the Aggrega host functions (`aggrega_log`, `aggrega_emit_warning`,
`aggrega_fetch_external_input`, the source and parse callbacks, and the
CLI output streams).
Each binding does its callback's allocation and encoding — a payload
document crosses as canonical CBOR, a bare string such as the fetch URL
as UTF-8 text — so a plugin calls a typed function and handles no wire
bytes.
The socket payload types themselves are generated from the canonical
schemas into the shared `aggrega-abi` package and re-exported by each
PDK, so the host, the guest codec and all three PDKs carry one wire
shape. Plugin authors targeting a mutator, policy, or any future
Aggrega socket consume the language-appropriate PDK and write
idiomatic source against it.

Contract floor: `v1alpha1`. The three PDKs export a common version
constant and stay coordinated at every socket-shape change.

## Layout

```text
aggrega-pdk/
├── README.md           (this file)
├── .github/workflows/
│   └── pdk.yml         (per-language build + test gates)
├── go/                 (Go PDK + examples + tests)
├── rust/               (Rust PDK + examples + tests)
└── typescript/         (TypeScript PDK + examples + tests)
```

Per-language authoring entry points live in `<language>/README.md`.
Start there for the language you author plugins in.

## Where the socket types come from

One artifact describes each socket shape, and it is the only one
authored: the **JSON Schema 2020-12** in the Aggrega schema
distribution, which is the wire-format contract crossing the
host/guest boundary.

From that schema a generator in the compiler writes the
**per-language codecs** — Go, Rust and TypeScript — into the shared
`aggrega-abi` package, where they are committed alongside it. Each
PDK re-exports the codec types for its own language rather than
declaring them: Go aliases them in `go/pkg/aggrega/abi_reexport.go`,
Rust re-exports them from `rust/src/lib.rs`, TypeScript from
`typescript/src/index.ts`. A Go alias is the same type, so a value a
plugin author builds against the `aggrega.<Type>` spelling is the
value the codec marshals — no conversion step, and no second
declaration that can drift.

Source-level textual equivalence between the host's in-memory
representation and a generated codec type is **not** required and
**not** verified; the wire format is the shared contract. Each
language's codec follows that language's idioms — Rust's `serde`
attributes, TypeScript's discriminated unions, Go's `json` tags —
while encoding to the same bytes, in the textual JSON encoding and the
canonical binary encoding alike.

Two gates hold the arrangement together:

- **Regeneration is byte-compared.** The compiler's own regeneration
  tests re-run the generator and fail when the committed codec bytes
  differ from what it now produces, so a schema edit that was never
  regenerated is loud rather than latent.
- **The three languages are checked against one corpus.** Shared
  golden vectors under `aggrega-abi/testdata/` are read by parity
  tests in each language, so Go, Rust and TypeScript are held to the
  same encoded bytes rather than to each other's word.

## Why one generated codec

The Aggrega contract is JSON Schema 2020-12, the same schema corpus
the compiler validates Catalog content against. Generating the codecs
from that corpus keeps one schema language end-to-end: there is no
second IDL (no `.proto`, no XTP Schema YAML, no service-definition
DSL) and no platform-CLI coupling sitting between plugin authors and
the typed surface they write against. The generator consumes the
schemas the project already maintains.

Generating rather than mirroring by hand is what keeps the fan-out
bounded. A socket-shape edit is one schema change plus a
regeneration, instead of the same edit repeated in each language; the
byte-comparison and the shared golden corpus above are what make a
missed regeneration or a diverging language fail rather than ship.

## Maintenance contract

Adding a new socket — or extending an existing one — touches the
following surfaces together, in one coordinated commit set:

1. The JSON Schema in the Aggrega schema distribution.
2. A regeneration of the `aggrega-abi` codecs, committed with the
   schema change.
3. The shared golden vectors under `aggrega-abi/testdata/`.
4. Each per-language PDK's builders and host-function bindings, where
   the socket adds an authored surface beyond the codec types.
5. Each per-language PDK's round-trip test fixture.
6. The host-side round-trip test fixture.
7. The integration test that exercises the loader end-to-end against
   the per-language fixtures.

A PDK CI workflow gates the per-language builds and tests; a
host-side integration test in the compiler's own test suite loads
the committed fixture `.wasm` artifacts and exercises them
end-to-end. A schema edit left unregenerated fails the compiler's
regeneration comparison; a codec that disagrees with its siblings
fails the shared golden-vector parity tests; and drift between a
committed `.wasm` fixture and the host's expectations fails the
integration test in the compiler's CI.

## Module-path identity

The published module paths all use the `aggrega-ai` namespace as
their identity:

- Go: `github.com/aggrega-ai/aggrega-pdk-go`
- Cargo: `aggrega-pdk-rust` (Cargo crate names are unscoped)
- npm: `@aggrega-ai/aggrega-pdk-typescript`

The module path is identity, not lookup URL. Module-path identity
anchors the public publication target; current source hosting is
orthogonal.

## License

The Aggrega Plugin Development Kits are released under the Apache License, Version 2.0; the full text is in the
[`LICENSE`](LICENSE) file at the root of this repository.
