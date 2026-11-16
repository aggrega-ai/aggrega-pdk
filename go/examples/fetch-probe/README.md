# fetch-probe (Go fixture)

A small Go plugin exercising the `aggrega_fetch_external_input` host
function. The `probe` export reads an external-input URL from its
input, fetches the URL's bytes through the PDK's `FetchExternalInput`
wrapper, and returns the fetched bytes as its output — or, on a fetch
failure, the failure message text, so host-side tests can assert the
in-band failure surface.

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

## Source-level dependency

This fixture consumes the PDK via a `replace` directive in `go.mod`
pointing at the PDK source in this repository at `../../`. The ABI module
it depends on resolves from the module proxy at the released version its
`go.mod` names. A plugin copied out of this repository drops the `replace`
and requires the published `github.com/aggrega-ai/aggrega-pdk/go` instead.
