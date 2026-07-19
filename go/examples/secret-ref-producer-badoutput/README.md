# secret-ref-producer-badoutput negative-path plugin (Go)

A deliberately contract-violating feature-recipe-pipeline step plugin authored
with the Go PDK for `GOOS=wasip1`. It exists to exercise the host's typed
output-contract enforcement — it is not a reference plugin.

Its `step` export declares a typed output contract requiring a string
`secretName`, but the guest returns a number for that field. The host validates
the guest's returned outputs against the declared schema after the step runs and
halts the build on the violation. A well-behaved producer returns outputs that
satisfy the schema it declares; this fixture proves the guardrail fires when one
does not.

## Build

```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o secret-ref-producer-badoutput-go.wasm ./
```

Or run `./build.sh`.
