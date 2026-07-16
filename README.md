# aggrega-pdk

Plugin Development Kits for authoring [Aggrega](https://aggrega.ai)
Wasm plugins. Three siblings live here, one per supported source
language:

- **`go/`** — Go PDK; targets publication to `pkg.go.dev` as
  `github.com/aggrega-ai/aggrega-pdk-go`.
- **`rust/`** — Rust PDK; targets publication to `crates.io` as
  `aggrega-pdk-rust`.
- **`typescript/`** — TypeScript PDK; targets publication to npm as
  `@aggrega-ai/aggrega-pdk-typescript`.

Each PDK is a thin, hand-authored wrapper over the corresponding
upstream [Extism](https://extism.org) PDK
(`extism-pdk-<language>`), adding Aggrega-typed builders for the
Wasm socket payloads and bindings for the Aggrega host functions
(`aggrega_log`, `aggrega_emit_warning`). Plugin authors targeting a
mutator, policy, or any future Aggrega socket consume the
language-appropriate PDK and write idiomatic source against it.

Contract floor: `v1alpha1`. The three PDKs export a common version
constant and stay coordinated at every socket-shape change.

## Layout

```text
aggrega-pdk/
├── README.md           (this file)
├── .github/workflows/
│   └── pdk.yml         (per-language build + test gates)
├── go/                 (Go PDK + examples + tests)
├── rust/               (Rust PDK + examples + tests, post-v1alpha1 ship)
└── typescript/         (TypeScript PDK + examples + tests, post-v1alpha1 ship)
```

Per-language authoring entry points live in `<language>/README.md`.
Start there for the language you author plugins in.

## Mirroring discipline

Three sources of truth describe each socket shape:

1. **Go canonical type** in the Aggrega compiler core (the host's
   in-memory representation).
2. **JSON Schema 2020-12** in the Aggrega schema distribution (the
   wire-format contract crossing the host/guest boundary).
3. **Per-language PDK type** in each subdirectory here (the
   author-facing surface in each source language).

All three are hand-authored. The JSON wire format the schema
describes is the shared contract joining them; source-level textual
equivalence between the Go canonical type and a PDK type is **not**
required and **not** verified. Each PDK type follows the target
language's idioms — Rust's `serde` attributes, TypeScript's
discriminated unions, Go's `json` tags — while serializing to the
same JSON shape.

Drift between any two mirrors is caught by round-trip tests on the
side that owns the mirror:

- A host-side round-trip test marshals a Go fixture, validates
  against the JSON Schema, and unmarshals back. Catches Go-struct ↔
  schema drift.
- Each PDK's tests do the equivalent: marshal a fixture in the PDK's
  package namespace, validate against the same JSON Schema, and
  unmarshal back. Catches PDK-type ↔ schema drift on the PDK side.

The schema is the pivot. A schema edit fans out to one Go canonical
type update and N PDK type updates; the per-language tests gate the
fan-out.

## Why hand-authored

The Aggrega contract is JSON Schema 2020-12, the same schema corpus
the compiler validates Catalog content against. Hand-authored PDKs
let the project reuse one schema language end-to-end: no second IDL
(no `.proto`, no XTP Schema YAML, no service-definition DSL), no
codegen toolchain, and no platform-CLI coupling sitting between
plugin authors and the typed surface they write against.

The trade-off is a bounded maintenance cost: a socket-shape edit
touches the Go canonical type, the JSON Schema, and three PDK type
modules at once. The round-trip tests make every drift loud at CI
time, so the cost is real but predictable — and small in absolute
terms because the socket count is small.

If, at a future re-evaluation, a codegen tool that consumes JSON
Schema 2020-12 directly and produces reusable library-shape output
(not full plugin scaffolds) becomes available, the hand-authored
PDKs can be replaced with generated ones without changing the
round-trip discipline: the same tests validate generated output the
same way they validate hand-authored output today.

## Maintenance contract

Adding a new socket — or extending an existing one — touches the
following surfaces together, in one coordinated commit set:

1. The Go canonical type in the Aggrega compiler core.
2. The JSON Schema in the Aggrega schema distribution.
3. Each per-language PDK's mirror type module.
4. Each per-language PDK's round-trip test fixture.
5. The host-side round-trip test fixture.
6. The integration test that exercises the loader end-to-end against
   the per-language fixtures.

A PDK CI workflow gates the per-language builds and tests; a
host-side integration test in the compiler's own test suite loads
the committed fixture `.wasm` artifacts and exercises them
end-to-end. Drift between the schema and a PDK fails the PDK's
round-trip test in PDK CI; drift between a committed `.wasm`
fixture and the host's expectations fails the integration test in
the compiler's CI.

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

Apache-2.0. See the `LICENSE` file in each per-language
subdirectory.
