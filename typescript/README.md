# aggrega-pdk-typescript

TypeScript PDK for authoring Aggrega Wasm plugins.

Package name: `@aggrega-ai/aggrega-pdk-typescript`.

The canonical reference for the plugin author's contract — what a plugin is,
the `describe()` self-report, exports and capability claims, runtime limits and
host functions, the `Plugin` entity, referencing a plugin, and the
publish → pull → build lifecycle — is the Wasm plugin developer guide in the
Aggrega documentation at [aggrega.ai](https://aggrega.ai). This README is the
TypeScript-specific quickstart; the guide is the single home for the concepts.

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

See `examples/mutator/` and `examples/policy/` for working
plugin sources with build scripts.

## Build provenance

A plugin can stamp its version and commit into its `describe()`
descriptor. Inject the values at the esbuild bundle step with `--define`:

```sh
npx esbuild src/index.ts --bundle --format=cjs --target=es2022 \
  --platform=neutral --outfile=dist/plugin.js \
  --define:AGGREGA_PLUGIN_VERSION='"1.2.3"' \
  --define:AGGREGA_PLUGIN_COMMIT='"abc1234"'
```

Build the descriptor with typed values and encode it through
`encodeDescriptor`; it folds the injected constants into the descriptor's
`metadata` object:

```ts
import { encodeDescriptor, type PluginDescriptor } from "@aggrega-ai/aggrega-pdk-typescript";
const d: PluginDescriptor = {
  abi_version: "v1alpha1",
  plugin_type: "cli",
  exports: [{ name: "cli_run" }],
  metadata: { author: "me" },
};
Host.outputString(encodeDescriptor(d));
```

`encodeDescriptor` reads the injected constants behind a `typeof` guard,
so a plain bundle (no `--define`) leaves the provenance empty and yields
no `version` / `commit` members rather than a `ReferenceError` — the
unstamped descriptor is byte-identical to the stamped one minus those
members.

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
    └── policy/ (fixture policy plugin)
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

The umbrella's `scripts/setup-workspace.sh` provisions `extism-js`,
binaryen, and Node, so a workspace set up through that script has the
fixture-build toolchain ready. For standalone checkouts, install the
tools manually per the pointers in the build.sh diagnostics.

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
