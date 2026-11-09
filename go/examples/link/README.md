# link (Go fixture)

A small Go plugin demonstrating the Aggrega cross-boundary edge-linking
exports. It declares the `cross_boundary_linking` capability and offers
`link_publish` and `link_resolve` as the paired contract the host routes a
cross-boundary `uses` edge through: `link_publish` derives a deterministic
pointer from the edge and records it on the consumed endpoint's assembled
file; `link_resolve` injects that pointer into the consuming endpoint's
assembled file. The mandatory `render_node` export is present and minimal.
Every export reads its input and writes its output in the socket's canonical
CBOR, through the payload types' `DecodeCBOR` and `EncodeCBOR`.

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
is gated on PDK publication.
