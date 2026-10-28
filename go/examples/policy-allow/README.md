# policy-allow (Go fixture)

A Go governance plugin for the governance socket's `ir-verdict` role
that drives the success path. Its `verdict_ir` export emits a constant
`PolicyOutput` with an empty verdict list and no `error` — the form of
"no rule fired" — so a host-side test can prove a build with an
`ir-verdict` attachment succeeds when the plugin reports nothing. It is
the counterpart to the `policy` fixture, whose `halt` verdict proves the
build-halt path. The guest does not read its input.

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

The build stamps a version into the guest via the linker, defaulting to
`1.0.0` and overridable with `AGGREGA_PLUGIN_VERSION`. The guest reports
that version through `describe()`, and the publish door cross-checks it
against a Plugin entity's `metadata.version`, so the two stay in lockstep.
