# aggrega-pdk-typescript

TypeScript PDK for authoring Aggrega Wasm plugins.

Package name: `@aggrega-ai/aggrega-pdk-typescript`.

## Authoring a plugin

A plugin is a small TypeScript project that depends on
`@aggrega-ai/aggrega-pdk-typescript` for the typed socket payloads
and `@extism/js-pdk` for the TypeScript declarations of the Extism
plugin runtime (`Host`, `Memory`, the `extism:host` ambient module).

The plugin entry follows the Extism JS PDK CommonJS convention:

- The exported function takes no parameters and returns nothing.
- Input flows in via `Host.inputString()` (or `Host.inputBytes()`).
- Output flows out via `Host.outputString(s)` (or `Host.outputBytes(b)`).
- Exports go through `module.exports = { ... }`.

Host functions exposed by the PDK (`log`, `emitWarning`) wrap the
ambient `extism:host` declaration + `Host.getFunctions()` destructuring
inside the wrapper body, so the PDK can be imported in non-Wasm
runtimes (the round-trip test suite) without trying to bind against a
Wasm-only host. See `src/host_funcs.ts` for the pattern.

Build the plugin to a `.wasm` artifact with `esbuild` + `extism-js`:

```sh
npx esbuild src/index.ts --bundle --format=cjs --target=es2022 \
  --platform=neutral --outfile=dist/plugin.js
extism-js -i index.d.ts dist/plugin.js -o plugin.wasm
```

The `extism-js` step compiles the bundled JS into a Wasm artifact
embedding the QuickJS runtime; typical artifact size is 2–3 MB. The
`index.d.ts` at the project root declares the plugin's exported
functions (under a `declare module "main"` block) plus the host
functions the plugin imports (under `declare module "extism:host"`).

See `examples/mutator/` and `examples/policy-validator/` for working
plugin sources with build scripts.

## Layout

```text
typescript/
├── README.md           (this file)
├── package.json        (@aggrega-ai/aggrega-pdk-typescript)
├── tsconfig.json       (strict + ES2022 + CommonJS + extism:host types)
├── .gitignore          (excludes node_modules/, dist/)
├── src/
│   ├── index.ts        (public-surface re-exports)
│   ├── version.ts      (API_VERSION constant)
│   ├── types.ts        (socket payload TypeScript interfaces)
│   └── host_funcs.ts   (ambient extism:host declaration + safe wrappers)
├── tests/
│   └── round_trip.test.ts (round-trip mirroring against the schemas)
└── examples/
    ├── mutator/        (fixture mutator plugin)
    └── policy-validator/ (fixture policy-validator plugin)
```

## Toolchain dependencies

The PDK and its fixture builds need three external tools beyond Node
+ npm:

- **`extism-js`** — the Extism JS PDK compiler binary (v1.6.0). Install
  from `github.com/extism/js-pdk/releases`; place at `~/.local/bin/extism-js`.
- **`wasm-merge`** + **`wasm-opt`** — from binaryen (v129). The
  `extism-js` compiler invokes these as subprocesses; both must be on
  PATH. Install from `github.com/WebAssembly/binaryen/releases`.
- **Node 22+** — for the `--experimental-strip-types` test runner +
  npm for fixture dependency installation.

The fixture `build.sh` scripts run a precondition check and fail fast
with install pointers if any of the three are missing.

The umbrella's `scripts/setup-workspace.sh` should grow `extism-js` +
binaryen install steps; until that lands, install manually per the
pointers in the build.sh diagnostics.

## Standalone reuse

The PDK's tests reference the JSON Schemas at relative path
`../../aggrega-schemas/json/` (relative to this package
root). This assumes the umbrella workspace layout. Standalone
consumption (after publication to npm) bundles the schemas in the
release artifact; the file-relative reference becomes
publication-bundled.

When running `npm test` from a checkout that does not have the
schemas alongside, the schema-dependent tests **fail** (Node's
built-in test runner has no dynamic-skip analog; an early `return`
would mark them as PASSED and mask the setup gap). Skip them
explicitly via:

```sh
node --test --experimental-strip-types --test-skip-pattern=round_trip tests/round_trip.test.ts
```

The umbrella's CI always has the schemas in place; the failure path
does not fire there.
