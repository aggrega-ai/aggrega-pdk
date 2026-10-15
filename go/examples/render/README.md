# render (Go fixture)

A small Go plugin demonstrating the Aggrega provider render socket. The
plugin decodes a `RenderNodeInput` from the socket's canonical CBOR payload
with `DecodeCBOR` and emits a single output file whose bytes are
`rendered-node:` followed by the node id, returned as the sole
`RenderedFile` of a `RenderNodeOutput` encoded with `EncodeCBOR`. The
transform matches the per-language render fixtures, so identical input
yields equivalent output whichever language rendered it.

Beside the file it reports two `RenderFact`s about what it emitted — the
paths it wrote (`emitted_paths`) and the node it echoed (`echoed_node`) —
in that order, which is deliberately not name order. Each value is a list the
guest writes itself as JSON, even the one holding a single entry; the echoed
node's entry is marshalled from a struct so its keys keep the order they are
declared in.

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
