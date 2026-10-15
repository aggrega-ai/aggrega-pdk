# policy-allow (Go fixture)

A Go plugin that drives the Aggrega policy socket's success
path. It emits a constant `PolicyOutput` whose single verdict
passes, so a host-side test can prove the socket lets a build succeed
when every rule passes — the counterpart to the minimal
`policy` fixture, whose deliberate failing verdict proves the
build-halt path.

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
