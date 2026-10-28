# policy (Go fixture)

A small Go governance plugin for the governance socket's `ir-verdict` role.
It offers two entry functions, so one guest covers both halves of a verdict
role's output contract:

- `verdict_ir` — the `ir-verdict` role's default entry function. Emits a
  `PolicyOutput` carrying two verdicts, in this order:
  - severity `halt`, message
    `fixture rule B: deliberate fail for end-to-end coverage`;
  - severity `info`, message `fixture rule A: advisory note`.
- `report_incomplete` — emits a `PolicyOutput` with no verdicts and the
  `error` message
  `fixture rule set could not be evaluated: no rule source was supplied`.
  This is the shape a verdict-role plugin returns when it ran but could
  not finish evaluating, and the host reads a present `error` as a failed
  invocation rather than as a verdict the plugin reached. Select it by
  naming it as the attachment's entry function:
  `function: report_incomplete`.

The guest does not read its input: both outputs are constants, for
end-to-end loader exercises.

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

## Source-level dependency

This fixture consumes the PDK via a `replace` directive in `go.mod`
pointing at the sibling PDK source at `../../`. The ABI module it
depends on resolves the same way, from the `aggrega-abi` repository
checked out beside this repository. Standalone reuse by end users
is gated on PDK publication.
