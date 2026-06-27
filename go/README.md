# aggrega-pdk-go

Go PDK for authoring Aggrega Wasm plugins.

Import path: `github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega`.

## Authoring a plugin

A plugin is a small Go module that imports `pkg/aggrega` for the typed
socket payloads and the Aggrega host-function bindings, plus
`github.com/extism/go-pdk` for the input/output byte plumbing.

Build the plugin to a `.wasm` artifact with the standard Go toolchain
targeting `GOOS=wasip1`:

```sh
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o <plugin>.wasm ./
```

The result is a WASI reactor. The Aggrega Wasm loader runs it under a
deterministic, capability-denied WASI configuration: the plugin's Go
runtime bootstraps against real WASI, but every value it can observe is
a deterministic function of the build input — observable wall time is
frozen, the entropy source yields zeros, stdio is discarded, and there
is no filesystem, argument, or environment access. The loader's static
import allowlist additionally rejects the filesystem-traversal and
socket WASI imports a plugin would need to reach outside this sandbox.

See `examples/mutator/` and `examples/policy-validator/` for working
plugin sources.

## Layout

```text
go/
├── README.md         (this file)
├── go.mod            (module github.com/aggrega-ai/aggrega-pdk-go)
├── pkg/aggrega/
│   ├── doc.go        (package doc)
│   ├── version.go    (APIVersion constant)
│   ├── types.go      (socket payload types — mirror the JSON Schemas)
│   ├── host_funcs.go (wasip1-build-tagged host function bindings)
│   ├── host_funcs_stub.go (host-build stubs for non-wasm targets)
│   └── aggrega_test.go (round-trip mirroring tests against the schemas)
└── examples/
    ├── mutator/      (fixture mutator plugin)
    └── policy-validator/ (fixture policy-validator plugin)
```

## Standalone reuse

The Go PDK's tests reference the JSON Schemas at relative path
`../../../aggrega-schemas/json/` (relative to `pkg/aggrega/`).
This assumes the umbrella workspace layout. Standalone consumption
(after publication to `pkg.go.dev`) bundles the schemas in the
release artifact; the file-relative reference becomes
publication-bundled. Until publication, only the umbrella checkout
runs the round-trip tests end-to-end; tests skip with a clear
diagnostic when the schemas are not reachable.
