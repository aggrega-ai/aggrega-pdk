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

### Starting from a skeleton

[`examples/governance-adapter-skeleton/`](examples/governance-adapter-skeleton)
is a copy-and-fill starting point for a governance plugin: one guest attachable
at every governance role, with no policy engine compiled in and one clearly
marked seam where an author drops their own evaluator. It is a skeleton rather
than a framework — the role wiring, the generated wire types and the pointer
escaping are done; the engine is not. Its README carries the role table, the four
things an adapter maps, and how to check the guest runs before attaching it.

### Values an input marks sensitive

A plugin never quotes, in a refusal or a warning it returns, a value its input
marks sensitive, by its key or as part of a payload marked as a whole: the host
carries a plugin's refusal and warning text to the reader unchanged, so the
plugin is the only party able to keep the value out. That includes text a library
writes for the plugin — a template engine's, a schema validator's or a parser's
failure over a marked value. The typed payloads carry the marking under its wire
names: on a provider's render input, `node.sensitive_rendered_keys` by key;
`node.sensitive_values`, `workload_projection.sensitive_workload`, each
`artifacts_projection.entries` element's `sensitive_content` and each `omits`
element's `sensitive` for a whole payload whose producing template read a
declared value; and on each `node.features` entry
read as the IR's feature entry, `sensitive_payload_leaves`, `sensitive_inject` and
`sensitive`; on the governance socket's `ir-verdict` input, the same attachment
fields and each node's `sensitive_payload_leaves` in the graph. The plugin
contract in the Aggrega documentation lists where each socket marks a key or a
payload, and what an absent marking does and does not say.

### The provider render socket

A provider guest's exports — `render_node`, and whichever optional exports its
`describe()` declares — read their input and write their output in canonical
CBOR: read `Host.inputBytes()` into the input type's `decodeCBOR…` function
(`decodeCBORRenderNodeInput` for `render_node`) and write the output with
`Host.outputBytes(encodeCBOR…(output).buffer as ArrayBuffer)`. A rendered file's
`content` holds the base64 of the bytes to write, and the encoder carries them as
raw bytes. [`examples/render-node/`](examples/render-node) and
[`examples/link/`](examples/link) are providers built this way.

### Reporting facts about what a provider emitted

A provider's `render_node` may return `render_facts` beside its files: an
ordered `RenderFact[]`, each a `name` and a `value`. Both are your provider's own
vocabulary, which you document for the authors who read them. **Report every
fact's `value` as an array, even when it holds one entry:** one name can be
reported by more than one render of a component — its own render calls and the
renders of the Features attached to it — and the host joins those lists into one
under the name, so it refuses a fact whose value is anything but a list and halts
the build. That is all the host learns of a value; it keys facts by name, hands
each list to the templates that read it, and writes the same files whether or not
facts accompany them. `JSON.stringify` writes an object's keys in the order they
were inserted, except that keys which read as array indexes come first in numeric
order, so give an entry's keys a fixed insertion order and avoid numeric-looking
keys where the order matters. `examples/render-node/` reports two facts.

### Leaving rendered units out

A source render may carry `omits` beside its `source_listing`: an ordered
`OmitEntry[]`, each a `kind`, a `name` and an optional `namespace`, already
resolved to plain strings. What an entry selects in your target's format is your
provider's own contract, which you document for the authors who write the
entries. A provider that honours them declares `omits: true` on its
`ProviderContract`, matches every entry against what it rendered, leaves each
matched unit out of everything it returns — its `files`, its `derived_keys` and
its `render_facts` — and returns `omitted`: one count per entry, in the order the
entries arrived, each the number of units that entry matched whether or not
another entry matched them too. A zero is a complete answer. A render handed no
entries is rendered exactly as it would be without the field, and leaves
`omitted` empty. An entry whose `sensitive` is set had at least one of its fields
produced from a value an author declared sensitive: match and count it like any
other, and keep every one of its fields out of a refusal, a warning or a log line,
as with every other value the input marks.

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
│   ├── index.ts        (public surface, re-exporting the abi types)
│   ├── version.ts      (API_VERSION constant)
│   ├── describe.ts     (descriptor helpers for the describe export)
│   ├── host_error.ts   (host-call error handling)
│   └── host_funcs.ts   (ambient extism:host declaration + safe wrappers)
├── tests/
│   └── round_trip.test.ts (round-trip tests against the schemas)
└── examples/           (one directory per worked plugin — a mutator, a
                        policy, a governance adapter skeleton, and others)
```

`types.ts` carries no interfaces: the socket payload types are
generated into the shared `@aggrega-ai/aggrega-abi` package and
re-exported from `index.ts`, so the names a plugin author imports are
the ones the codec serializes. See the [kit overview](../README.md) for
the generation and the gates that hold it.

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
schemas alongside, the schema-dependent tests **fail** with a clear
diagnostic, because a run that could not read them has verified
nothing. Where they are genuinely absent — a standalone checkout of
this kit, or a packaging test — opt out with the same switch the Go
and Rust kits use:

```sh
AGGREGA_SKIP_SCHEMA_TESTS=1 npm test
```

Each affected test is then reported as skipped, with the reason
stated. The switch keys on the schemas being unreachable: with them in
place it changes nothing, so it cannot quietly turn a real run into a
skipped one.

`npm test` first builds, which resolves this package's dependency on
the shared ABI package from a sibling directory. Where that sibling is
also absent, run the test file directly so the build step is not
involved — the switch behaves the same way:

```sh
AGGREGA_SKIP_SCHEMA_TESTS=1 node --test --experimental-strip-types \
  tests/round_trip.test.ts
```

The umbrella's checkout always has the schemas in place, so neither
the refusal nor the skip fires there.
