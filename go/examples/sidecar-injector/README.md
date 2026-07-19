# sidecar-injector reference plugin (Go)

A reference cross-entity feature-recipe-pipeline step plugin authored with the
Go PDK and built with the standard Go toolchain for `GOOS=wasip1`. It
self-describes as an `frp_step` plugin via `describe()` and implements a real
`step` export.

It PATCHes the consuming (host) workload to add a companion container: it
content-selects the host chart's `values.yaml` and adds a named member under its
`containers` map (an RFC 6902 `add` of a structured value). This is the
additive-sidecar shape — a Feature that places a companion container alongside
the consumer's own workload.

Its `step` export declares `operates=patch`, `scope=host`, and the `helm-chart`
compatible provider. It declares no typed input contract: the container it
appends is fixed, so it reads no step-local input.

Selection is by content, never by filename: the guest learns each unit's key at
runtime and roots its `/host` PATCH at the `values.yaml` unit.

## Build

```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o sidecar-injector-go.wasm ./
```

Or run `./build.sh`.
