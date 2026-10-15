# mutator (Rust fixture)

A small Rust plugin demonstrating the Aggrega mutator socket. The
plugin reads a `MutatorInput` payload, appends a known label to every
node, and emits a `MutatorOutput` with one JSON Patch per node.

The socket's wire carries canonical CBOR in both directions, read and
written through the generated codec on the payload type
(`MutatorInput::decode_cbor(...)` and `out.encode_cbor()`), so the export
takes and returns `Vec<u8>` rather than `String`. The patch documents the
output carries are JSON: the socket carries them verbatim and neither
side reads into them.

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
the PDK via a path dependency. The PDK in turn takes the ABI crate by
path from the `aggrega-abi` repository checked out beside this
repository. Standalone reuse by end users is gated on PDK publication.
