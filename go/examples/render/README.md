# render (Go fixture)

A small Go plugin demonstrating the Aggrega provider render socket. The
plugin reads a `RenderNodeInput` JSON payload and emits a single output
file whose bytes are `rendered-node:` followed by the node id, returned
as the sole `RenderedFile` of a `RenderNodeOutput`. The transform matches
the per-language render fixtures, so identical input yields equivalent
output whichever language rendered it.

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
