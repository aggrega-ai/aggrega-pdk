# policy (Go fixture)

A small Go plugin demonstrating the Aggrega policy socket. It offers
two entry points, so one guest covers both halves of the socket's
output contract:

- `verdict_ir` — the socket's default entry point. Emits a deterministic
  two-result `PolicyOutput`: one halt verdict and one info verdict.
- `report_incomplete` — emits a `PolicyOutput` with no verdicts and an
  `error` message. This is the shape a policy plugin returns when it
  ran but could not finish evaluating, and the host reads a present
  `error` as a failed invocation rather than as a verdict the plugin
  reached. Select it by naming it as the attachment's entry function.

Both outputs are constants, for end-to-end loader exercises.

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

The build stamps a version into the guest via the linker, defaulting to
`1.0.0` and overridable with `AGGREGA_PLUGIN_VERSION`. The guest reports
that version through `describe()`, and the publish door cross-checks it
against a Plugin entity's `metadata.version`, so the two stay in lockstep.

## Source-level dependency

This fixture consumes the PDK via a `replace` directive in `go.mod`
pointing at the sibling PDK source at `../../`. The umbrella checkout
provides the file-relative resolution. Standalone reuse by end users
is gated on PDK publication.
