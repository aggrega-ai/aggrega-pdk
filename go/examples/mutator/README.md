# mutator (Go fixture)

A small Go plugin demonstrating the Aggrega mutator socket. The plugin
reads a `MutatorInput` payload, appends a known label to every node, and
emits a `MutatorOutput` with one JSON Patch per node.

The socket's wire carries canonical CBOR in both directions, read and
written through the generated codec on the payload type
(`in.DecodeCBOR(...)` and `out.EncodeCBOR()`). The patch documents the
output carries are JSON: the socket carries them verbatim and neither
side reads into them.

## Build

Standard Go required (the build targets `GOOS=wasip1`). Run from this
directory:

```sh
bash build.sh
```

Destination resolution:

1. `$AGGREGA_PDK_WASM_OUT` if set — explicit override (the `aggrega-cli`
   test harness sets it when it builds this example as a fixture, staging
   into a temporary directory).
2. Otherwise, the PDK's own `testdata/` directory.

## Source-level dependency

This fixture consumes the PDK via a `replace` directive in `go.mod`
pointing at the sibling PDK source at `../../`. The ABI module it
depends on resolves the same way, from the `aggrega-abi` repository
checked out beside this repository. Standalone reuse by end users
is gated on PDK publication (planned post-`v1alpha1`).
