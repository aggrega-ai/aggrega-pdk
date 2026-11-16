# policy (Rust fixture)

A small Rust governance plugin for the governance socket's `ir-verdict`
role. Its `verdict_ir` export emits a `PolicyOutput` carrying two verdicts,
in this order:

- severity `halt`, message
  `fixture rule B: deliberate fail for end-to-end coverage`;
- severity `info`, message `fixture rule A: advisory note`.

The guest does not read its input: the output is a constant, for
end-to-end loader exercises.

## Build

Rust toolchain with the `wasm32-unknown-unknown` target required.
Run from this directory:

```sh
bash build.sh
```

Destination resolution:

1. `$AGGREGA_PDK_WASM_OUT` if set — explicit override (the `aggrega-cli`
   test harness sets it when it builds this example as a fixture, staging
   into a temporary directory).
2. Otherwise, the PDK's own `testdata/` directory.

## Source-level dependency

This fixture is a workspace member of `aggrega-pdk-rust` and consumes
the PDK via a path dependency. The PDK in turn takes the ABI crate from
crates.io at the released version its manifest names. A plugin copied out
of this repository depends on the published `aggrega-pdk-rust` crate
instead of the path.
