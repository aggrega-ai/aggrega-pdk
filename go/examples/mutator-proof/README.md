# mutator-proof (Go fixture)

A Go plugin that drives the Aggrega mutator socket end-to-end. It reads a
`MutatorInput` payload and emits one JSON Patch per node that appends the
label `proof.aggrega/touched=mutator-proof`. Unlike the minimal `mutator`
fixture, this plugin escapes the label key per RFC 6901 (`/` becomes
`~1`), so the patch applies cleanly to the host's labels-only projection
and the injected label reaches the IR graph. It exists so a host-side
test can prove the socket wires through.

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
