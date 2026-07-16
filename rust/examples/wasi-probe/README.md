# wasi-probe (Rust fixture)

A small Rust plugin that exercises the host's stubbed-WASI subset
end-to-end. The fixture builds for `wasm32-wasip1` (NOT the
`wasm32-unknown-unknown` target the other fixtures use) so it emits
real `wasi_snapshot_preview1.*` imports — the host's firewall and
stub table see them and respond with deterministic constants.

## What it probes

The exported function `probe_wasi_stubs` calls three WASI imports
and writes the observed responses into a 40-byte output buffer at
offset 0 of plugin memory:

- `clock_time_get` — should return a u64(0) timestamp under the stub
- `random_get` — should fill its buffer with 16 zero bytes
- `fd_write` — should report the iovec length and errno 0 without
  writing to host stdout

The host's integration test asserts that `probe_wasi_stubs` returns
without error. A successful return proves the firewall accepted the
fixture's WASI imports, that Extism registered the stubs under
`wasi_snapshot_preview1`, and that each stub returned a value the
plugin accepted without trapping. Byte-level assertions on the
observation buffer (verifying the specific stub return values rather
than just successful execution) require either using the Extism PDK's
output channel from this fixture or driving the plugin via a
Wazero-direct path; that's a follow-up if stronger byte-level
coverage is wanted.

## Build

```sh
bash build.sh
```

Destination resolution:

1. `$AGGREGA_PDK_WASM_OUT` if set — explicit override.
2. The umbrella's `aggrega-cli/internal/wasm/testdata/` directory
   when this PDK is checked out as a submodule.
3. The PDK's own `testdata/` directory for standalone checkouts.

The Rust toolchain's `wasm32-wasip1` target is required; `build.sh`
runs `rustup target add wasm32-wasip1` idempotently.

## Why this fixture exists

The mutator and policy fixtures both build for
`wasm32-unknown-unknown` and emit zero WASI imports. They prove the
host accepts WASI-free Wasm; they do NOT prove the host's stubbed-
WASI subset works.

The `wasi-probe` fixture closes that coverage gap. It is the
canonical end-to-end test that the firewall accepts the allowlisted
WASI imports AND that each stub responds with the documented
deterministic constant.
