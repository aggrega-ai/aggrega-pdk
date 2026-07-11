# link (Go fixture)

A small Go plugin demonstrating the Aggrega cross-boundary edge-linking
exports. It declares the `cross_boundary_linking` capability and offers
`link_publish` and `link_resolve` as the paired contract the host routes a
cross-boundary `uses` edge through: `link_publish` derives a deterministic
pointer from the edge and records it on the consumed endpoint's assembled
file; `link_resolve` injects that pointer into the consuming endpoint's
assembled file. The mandatory `render_node` export is present and minimal.

## Build

Standard Go required (the build targets `GOOS=wasip1`). Run from this
directory:

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

This fixture consumes the PDK via a `replace` directive in `go.mod`
pointing at the sibling PDK source at `../../`. The umbrella checkout
provides the file-relative resolution. Standalone reuse by end users
is gated on PDK publication (planned post-`v1alpha1`).
