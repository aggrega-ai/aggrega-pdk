# frp-step reference plugin (Go)

A reference feature-recipe-pipeline step plugin authored with the Go PDK
and built with the standard Go toolchain for `GOOS=wasip1`. It
self-describes as a `pipeline-step` plugin via `describe()` and implements a
real `step` export.

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
standalone checkout) with
`GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared`. Requires the
standard Go toolchain.

The build stamps a version into the guest via the linker, defaulting to
`1.0.0` and overridable with `AGGREGA_PLUGIN_VERSION`. The guest reports
that version through `describe()`, and the publish door cross-checks it
against a Plugin entity's `metadata.version`, so the two stay in lockstep.

The plugin is a WASI reactor: its Go runtime bootstraps under the Aggrega
loader's deterministic, capability-denied WASI configuration, then
`step` decodes the keyed document set and emits its patches and side
effects. The pipeline-step socket carries canonical CBOR in both
directions, so `step` reads its input with `DecodeCBOR` and writes its
output with `EncodeCBOR`, through the generated ABI codec.
