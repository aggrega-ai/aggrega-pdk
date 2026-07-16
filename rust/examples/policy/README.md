# policy (Rust fixture)

A small Rust plugin demonstrating the Aggrega policy socket.
The plugin emits a deterministic two-result `PolicyOutput` —
one pass, one fail — for end-to-end loader exercises.

## Build

Rust toolchain with the `wasm32-unknown-unknown` target required.
Run from this directory:

```sh
bash build.sh
```

Destination resolution:

1. `$AGGREGA_PDK_WASM_OUT` if set — explicit override (the umbrella's
   integration test driver uses this when staging into a temporary
   directory).
2. The umbrella's `aggrega-cli/internal/wasm/testdata/` directory when
   this PDK is checked out as a submodule of the umbrella.
3. The PDK's own `testdata/` directory for standalone checkouts.

## Source-level dependency

This fixture is a workspace member of `aggrega-pdk-rust` and consumes
the PDK via a path dependency. The umbrella checkout provides the
sibling structure. Standalone reuse by end users is gated on PDK
publication.
