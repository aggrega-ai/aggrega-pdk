# frp-step reference plugin (Go)

A reference feature-recipe-pipeline step plugin authored with the Go PDK
and built with TinyGo. It self-describes as an `frp_step` plugin via
`describe()` and implements a real `step` export.

The plugin exercises the pipeline contract end-to-end:

- It content-selects a workload from the keyed document set — a document
  whose `kind` is `Deployment` — learns that document's key at runtime,
  and roots an own-scope PATCH at it, adding an annotation.
- It EMITs a new own-scope `ConfigMap` document.
- It EMITs a host-scope marker document when the host document set is
  present (a component-scope FeatureRecipe attached to a host).

Its `step` export declares `operates=patch`, `scope=own`, and the `helm`
and `helm-chart` compatible providers. The host reads `describe()` once at
load, validates the descriptor, then dispatches `step` with the keyed
document set as input.

This plugin is the Go-PDK counterpart of the Rust `frp-step-patch`
reference plugin; the two implement the same behaviour against the same
pipeline contract.

## Build

```sh
./build.sh
```

Builds `frp-step-go.wasm` into the umbrella's
`aggrega-cli/internal/wasm/testdata/` (or the PDK's own `testdata/` for a
standalone checkout). Requires TinyGo on `PATH`.

The build selects a non-leaking garbage collector
(`-gc=conservative`). The `wasm-unknown` target's default leaking
collector breaks `encoding/json`'s reflective decode of the keyed
document set, so a real-decode plugin must override it.
