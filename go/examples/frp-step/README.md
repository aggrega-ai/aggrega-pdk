# frp-step fixture plugin (Go)

A minimal feature-recipe-pipeline step plugin used by the host's
plugin-load-gate tests. It self-describes as a `frp_step` plugin via
`describe()` and implements a no-op `step` export.

The fixture proves the host's load-and-dispatch path end-to-end: load
the module, call `describe()`, validate the returned descriptor, then
dispatch the declared export. It does **not** implement real step
behaviour (AST patches, side effects) — that is the reference plugin's
job.

## Build

```sh
./build.sh
```

Builds `frp-step-go.wasm` into the umbrella's
`aggrega-cli/internal/wasm/testdata/` (or the PDK's own `testdata/` for
a standalone checkout). Requires TinyGo on `PATH`.
