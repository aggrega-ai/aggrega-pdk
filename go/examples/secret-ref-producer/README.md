# secret-ref-producer reference plugin (Go)

A reference feature-recipe-pipeline step plugin authored with the Go PDK and
built with the standard Go toolchain for `GOOS=wasip1`. It self-describes as an
`pipeline-step` plugin via `describe()` and implements a real `step` export.

It stands at the head of a producer-to-injector chain. In one step it:

- EMITs a secret-reference document into its own document set — a small manifest
  naming a secret, which the runtime environment's controller resolves at deploy
  time.
- RETURNS an addressable output naming that secret, so a later injector step
  reads the name through the pipeline's inter-step channel as
  `.steps.<id>.outputs.secretName`.

Its `step` export declares `operates=emit`, `scope=own`, the `helm` compatible
provider, and a typed output contract — a JSON Schema requiring a string
`secretName`. The host reads `describe()` once at load and validates the guest's
returned outputs against that schema after the step runs, so an output that
omits the field or gives it a non-string value halts the build.

## Build

```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o secret-ref-producer-go.wasm ./
```

Or run `./build.sh`, which resolves the output directory (the umbrella's
Wasm testdata directory when checked out as a submodule) and builds there.
