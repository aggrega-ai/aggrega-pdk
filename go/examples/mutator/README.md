# mutator (Go fixture)

A small Go plugin demonstrating the Aggrega mutator socket. The plugin
reads a `MutatorInput` JSON payload, appends a known label to every
node, and emits a `MutatorOutput` with one JSON Patch per node.

## Build

TinyGo required. Run from this directory:

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
