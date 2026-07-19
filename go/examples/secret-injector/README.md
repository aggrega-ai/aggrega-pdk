# secret-injector reference plugin (Go)

A reference cross-entity feature-recipe-pipeline step plugin authored with the
Go PDK and built with the standard Go toolchain for `GOOS=wasip1`. It
self-describes as an `frp_step` plugin via `describe()` and implements a real
`step` export.

It is the downstream half of a producer-to-injector chain. It reads a secret
name an earlier producer step supplied through the rendered `with` block — the
pipeline author wires it from `.steps.<id>.outputs.secretName` — and PATCHes the
consuming (host) workload to reference that secret: it content-selects the host
chart's `values.yaml` and adds the secret name under the workload's
`environment` block.

Its `step` export declares `operates=patch`, `scope=host`, the `helm-chart`
compatible provider, and a typed input contract — a JSON Schema requiring a
string `secretName`. The host reads `describe()` once at load and validates the
rendered `with` input against that schema before the guest runs, so a missing or
wrong-typed `secretName` halts the build.

Selection is by content, never by filename: the guest learns each unit's key at
runtime and roots its `/host` PATCH at the `values.yaml` unit (a structured
object carrying an `image` key and no `apiVersion`).

## Build

```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o secret-injector-go.wasm ./
```

Or run `./build.sh`.
