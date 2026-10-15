# mutator-proof (Go fixture)

A Go plugin that drives the Aggrega mutator socket end-to-end. It reads a
`MutatorInput` payload and emits one JSON Patch per node that appends the
label `proof.aggrega/touched=mutator-proof`. Unlike the minimal `mutator`
fixture, this plugin escapes the label key per RFC 6901 (`/` becomes
`~1`), so the patch applies cleanly to the host's labels-only projection
and the injected label reaches the IR graph. It exists so a host-side
test can prove the socket wires through.

The socket's wire carries canonical CBOR in both directions, read and
written through the generated codec on the payload type. This plugin
needs only the node identifiers, and it still reads them through the
shared payload type rather than a local restatement of it, so a change to
the wire shape reaches this fixture. The patch documents it emits are
JSON: the socket carries them verbatim.

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
