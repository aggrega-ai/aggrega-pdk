# mutator (TypeScript fixture)

A small TypeScript plugin demonstrating the Aggrega mutator socket.
The plugin reads a `MutatorInput` JSON payload, appends a known
label to every node, and emits a `MutatorOutput` with one JSON
Patch per node.

## Build

Required tools: Node 22+, `extism-js` (v1.6.0 line), `wasm-merge`,
`wasm-opt`. Run from this directory:

```sh
bash build.sh
```

The build:

1. Verifies the four required tools are on PATH (fails fast with
   install pointers if any is missing).
2. Resolves the artifact destination directory (three-tier order:
   `$AGGREGA_PDK_WASM_OUT` → umbrella superproject's testdata →
   PDK-root testdata).
3. `npm install --no-save` brings esbuild + types into a fixture-
   local `node_modules/` without writing a lockfile.
4. `esbuild` bundles `src/index.ts` to a single CommonJS JS file at
   `dist/plugin.js`.
5. `extism-js -i index.d.ts dist/plugin.js` compiles the bundled JS
   into a `.wasm` artifact embedding the QuickJS runtime. Typical
   artifact size: ~2–3 MB.

## Plugin entry shape

The fixture's `src/index.ts` follows the Extism JS PDK CommonJS
convention: `function mutate(): void`, input via `Host.inputString()`,
output via `Host.outputString(...)`, exposed via `module.exports`.
The `index.d.ts` at this directory's root declares the exported
function for `extism-js` to bind (`declare module "main"`) and the
host functions the plugin imports (`declare module "extism:host"`,
empty `user` interface since this fixture doesn't call any host
functions).
