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
pointing at the sibling PDK source at `../../`, plus a matching
`replace` for the ABI sibling (a dependency module's own replace
directives do not apply from a consuming main module). That second
`replace` resolves from the `aggrega-abi` repository checked out beside
this repository. Standalone reuse by end users is gated on PDK
publication.
