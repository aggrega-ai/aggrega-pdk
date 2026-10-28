# policy (TypeScript fixture)

A small TypeScript governance plugin for the governance socket's
`ir-verdict` role. Its `verdict_ir` export emits a `PolicyOutput`
carrying two verdicts, in this order:

- severity `halt`, message
  `fixture rule B: deliberate fail for end-to-end coverage`;
- severity `info`, message `fixture rule A: advisory note`.

These are the same two verdicts, in the same order, as the `verdict_ir`
export of the Go and Rust `policy` examples.

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
convention: `function verdict_ir(): void`, input via `Host.inputBytes()`
(discarded — the fixture's contract is constant output), output via
`Host.outputBytes(...)` carrying the result `encodeCBORPolicyOutput` writes,
since the role's wire is canonical CBOR, exposed via `module.exports`. The
`index.d.ts` at this directory's root declares the exported function
for `extism-js` to bind.
