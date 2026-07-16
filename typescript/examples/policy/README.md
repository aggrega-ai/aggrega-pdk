# policy (TypeScript fixture)

A small TypeScript plugin demonstrating the Aggrega policy
socket. The plugin emits a deterministic two-result
`PolicyOutput` — one pass, one fail — matching the Go and
Rust counterparts byte-for-byte at the verdict level.

## Build

Required tools: Node 22+, `extism-js` (v1.6.0 line), `wasm-merge`,
`wasm-opt`. Run from this directory:

```sh
bash build.sh
```

See the mutator fixture's README for the full build flow; the
policy fixture follows the same shape.

## Plugin entry shape

The fixture's `src/index.ts` follows the Extism JS PDK CommonJS
convention: `function validate(): void`, input via `Host.inputString()`
(discarded — the fixture's contract is constant output), output via
`Host.outputString(...)`, exposed via `module.exports`. The
`index.d.ts` at this directory's root declares the exported function
for `extism-js` to bind.
