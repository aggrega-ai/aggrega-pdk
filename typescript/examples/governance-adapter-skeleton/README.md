<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2026 Cognitio S.r.l.
-->

# governance-adapter-skeleton

A skeleton for a governance plugin in TypeScript. Copy this directory, drop your
policy engine into the seam, and the result is a guest an operator can attach at
every governance role.

**This is a skeleton, not a framework.** No policy engine is compiled in and no
policy language is assumed. Everything specific to an engine lives behind one
function; the rest is the wiring that stays the same whatever engine is behind
it.

The canonical reference for what a plugin is, the `describe()` self-report,
runtime limits and host functions, the `Plugin` entity and the
publish → pull → build lifecycle is the Wasm plugin developer guide in the
Aggrega documentation at [aggrega.ai](https://aggrega.ai). This README is the
skeleton's own guide.

## The four roles, and what each one may do

A governance plugin is one guest attached at one of four roles. The role an
operator attaches it at — not anything the guest says about itself — fixes which
payload arrives and what the guest may emit.

| Role | Export | What arrives | What may be emitted |
| --- | --- | --- | --- |
| `annotation-transform` | `transform_annotations` | the components that may be annotated, plus the annotation keys the deployment has registered a meaning for | annotations to add to components that already exist |
| `label-transform` | `mutate` | the build's nodes with the labels they currently carry, plus the deployment's name and environment | label changes, one per decision |
| `ir-verdict` | `validate` | the build graph, plus the routing paths enumerated alongside it | verdicts |
| `render-verdict` | `validate` | the rendered output files, each with its text and, where the host parsed it, a structured view of its content | verdicts |

One `validate` export serves both verdict roles: the two payloads identify
themselves by shape, so the guest reads what it was handed rather than declaring
which role it expects.

The two transform roles change the build; the two verdict roles report an outcome
and the build never applies their output as a change. An operator who attaches a
plugin at a transform role has declared that transformation — that is what makes
it a change the build accepts rather than one nobody asked for.

## The four things an adapter maps

Whatever engine sits behind the seam, an adapter is wiring four things:

- **sources become the engine's program text.** The attachment's files arrive
  verbatim, sorted by path, each with its base name and its content. Nothing
  outside the guest has parsed them.
- **settings become the engine's data**, plus any tunable the adapter itself
  needs. The settings object arrives verbatim too, and is `undefined` when the
  attachment declares none.
- **the input is the payload of the attached role** — the four in the table
  above.
- **the output is either the verdicts the engine reached, or the changes a
  transform role asks for.**

## The seam

`evaluate` in [`src/evaluator.ts`](src/evaluator.ts) is the one module a new
adapter has to rewrite:

```ts
export function evaluate(
  programs: Program[],
  settings: unknown,
  payload: string,
  decision: string,
): unknown[];
```

It receives the programs, the settings, the whole role payload, and the name of
the decision the caller wants back. It returns one value per decision.

Handing over the whole payload rather than a projection of it is deliberate: a
policy then reads the graph, the artifacts, the nodes or the targets under the
names the wire already gives them, so the input a policy author writes against is
learnable from one place.

Two obligations on whatever replaces the body:

- **Identical input must yield an identical sequence of decisions.** A build's
  output is byte-identical across runs of identical input, and an adapter that
  iterated an unordered collection would break that from inside the guest. Where
  an engine yields a set, sort it before returning. `Object.keys` returns
  insertion order, which is stable for one input and not across inputs, so sort
  it rather than relying on it.
- **A failure to evaluate must throw, never return an empty result.** An empty
  result is a policy that found nothing to say; a thrown failure is a policy that
  did not run. Reporting the second as the first lets a build through unexamined.

## The decisions each role asks for

Each role asks the seam for its own decision, and the names are the adapter's own
choice rather than part of the wire:

| Role | Decision | Shape of one decision |
| --- | --- | --- |
| `annotation-transform` | `annotations` | `{"boundary_path", "component_name", "annotation_key", "annotation_value"}` |
| `label-transform` | `labels` | `{"node", "key", "value"}`, and optionally `"op"` as `"add"` or `"remove"` |
| `ir-verdict` / `render-verdict` | `deny`, `warn`, `audit` | the message text the build reports |

### Verdict decisions and wire severities are different vocabularies

`deny`, `warn` and `audit` are decision collections — a convention many policy
languages already group decisions under, adopted here so a policy author can
reuse what they know. An adapter is free to pick other names, or to read them
from the attachment's settings so an operator can choose.

The severities the wire carries are a separate, closed set of three: `halt` stops
the build and names what to fix, `warn` surfaces alongside a build that still
completes, and `info` records the observation and leaves the build's outcome
unchanged. `COLLECTIONS` in [`src/verdicts.ts`](src/verdicts.ts) is the mapping
between the two, and keeping them apart is what lets a policy author rename a
decision collection without changing what the build does with it.

### How a failure travels, and the two channels it can take

Both the transform and the verdict outputs carry an `error` member alongside their
result, and the build reads a present one as a failed invocation: the guest ran,
produced a well-formed output document, and named in it a failure it could not
recover from. That is distinct from the diagnostics a verdict carries — a
diagnostic reports something the policy found during a run that still produced a
result, while `error` reports that the run itself did not complete.

The other channel is the guest call failing outright, which the build also reads
as a failed invocation. This skeleton takes that one: a role that cannot read its
payload, or whose engine could not evaluate, fails the call with a message naming
the role. One channel rather than two is deliberate — two paths to the same
outcome is two places to keep in step — and it is the channel the shipped policy
guests use.

Fill in `error` instead if your engine distinguishes "evaluated, and here is what
went wrong" from "could not run at all" and you want the first reported with a
partial result beside it. What must not happen either way is a failure reported as
an empty result: on a governance surface an empty result reads as a policy that
allowed the build.

### A policy names the label; the guest builds the path

The label role's output is a document of edit instructions addressed by path, and
those paths need escaping — a label key routinely contains a `/`, which is the
path separator, so a hand-written path would address a nested location and the
change would be refused. A decision therefore names the label it means, and
`src/labels.ts` builds the path. The escaping is written once rather than in every
policy.

### A decode is a typed view, not a check

This is the one place a TypeScript adapter needs more care than its Go or Rust
siblings. The generated decode functions — either encoding's — read the payload
and give it a type; they do not validate it, because TypeScript's types are
erased before the guest runs. A role that skipped the check would read `undefined` where it expected a
list, iterate nothing, and emit an empty result — reporting "the policy found
nothing to say" for a payload it never understood.

[`src/shape.ts`](src/shape.ts) is that check: each role confirms the member it
depends on before reading it, and a payload the guest cannot read is refused
naming the role and the member.

## Layout, and what to delete

```text
governance-adapter-skeleton/
├── README.md                  (this file)
├── package.json
├── tsconfig.json
├── index.d.ts                 (the exports extism-js compiles in)
├── build.sh                   (build to dist/)
└── src/
    ├── index.ts               (the describe() self-report and the exports)
    ├── evaluator.ts           (THE SEAM — the one module a new adapter rewrites)
    ├── annotations.ts         (the annotation-transform role)
    ├── labels.ts              (the label-transform role)
    ├── verdicts.ts            (the ir-verdict and render-verdict roles)
    ├── shape.ts               (refusing a payload this guest cannot read)
    └── extism-globals.d.ts    (the module.exports object the runtime supplies)
```

One module per role, so the unit of deletion is the role. An adapter that only
renders verdicts deletes `annotations.ts` and `labels.ts`, their imports, their
entries in `module.exports` and in `index.d.ts`, and the two matching entries in
`descriptor()`. The host refuses to call an export the guest did not declare, so
an attachment pointed at a role the adapter does not serve is refused by name
rather than silently doing nothing.

## Wire types come from the generated codec

Every payload here is a generated type with a generated JSON codec, generated from
the same schemas the host reads, so the guest and the host name one shape and
neither can drift from the other. Read a payload with `decode…` and write a result
with `encode…`; do not hand-write an interface for a payload, because a
hand-written one is a second copy of the shape and the copy is what goes out of
step.

The codec ships two pairs, one per wire encoding, and **which pair a role uses is
fixed by that role's wire rather than by preference**. The `label-transform` role's
wire carries canonical CBOR, so `src/labels.ts` reads with
`decodeCBORMutatorInput` and writes with `encodeCBORMutatorOutput`, and its export
reads and writes bytes (`Host.inputBytes()` / `Host.outputBytes(...)`). The other
three roles carry JSON and use the `decode…` / `encode…` pair with the string
forms. A guest serving several roles is therefore mixed on purpose: each export
reads its own payload, so the roles move independently.

## Build

```sh
./build.sh
```

The artifact lands at `dist/governance-adapter-skeleton-typescript.wasm`. Set
`AGGREGA_PDK_WASM_OUT` to stage it elsewhere.

The build bundles the sources with `esbuild` and compiles the bundle with
`extism-js`, which embeds a JavaScript runtime in the artifact. `build.sh` checks
for `node`, `npm`, `extism-js`, `wasm-merge` and `wasm-opt` up front and names
whichever is missing.

## Check it runs, before attaching it

`aggrega plugin run-local` loads a guest from a path, checks the export against
the guest's own `describe()` self-report, calls it, and writes the response to
standard output. One call per role:

```sh
aggrega plugin run-local --plugin dist/governance-adapter-skeleton-typescript.wasm \
  --type governance --export transform_annotations --with @annotation-payload.json

aggrega plugin run-local --plugin dist/governance-adapter-skeleton-typescript.wasm \
  --type governance --export mutate --with @label-payload.json

aggrega plugin run-local --plugin dist/governance-adapter-skeleton-typescript.wasm \
  --type governance --export validate --with @ir-verdict-payload.json

aggrega plugin run-local --plugin dist/governance-adapter-skeleton-typescript.wasm \
  --type governance --export validate --with @render-verdict-payload.json
```

The response reaches you exactly as the guest emitted it, so the per-role wires
above are what you see here, and these calls legitimately differ: the `mutate`
call prints bytes, and the rest print readable text.

An unmodified skeleton reaches no engine, so each role emits its own neutral
result — no annotations, no label changes, and one `info` note per verdict role
naming the programs that reached the seam. That note is the wiring proof: the
guest decoded the payload the host handed it, ran, and emitted the shape its role
is allowed to emit. It comes from the placeholder in `src/evaluator.ts` and goes
when the engine arrives.

A payload the guest cannot read is refused with a message naming the role, rather
than evaluated as an empty result.

## Attach it

An operator attaches the guest at a role in the governance socket configuration.
Each attachment names its role, and may carry the files the policy is written in
and a settings object:

```yaml
plugins:
  governance:
    - uri: <resolvable-plugin-reference>
      role: ir-verdict
      with:
        sources:
          - rules.policy
        settings:
          max_nodes: 50
```

The same guest may be attached more than once, at different roles, with different
files and settings.

## Copying it out

Two edits before this becomes your adapter: replace the license header in each
file with your own, and rename the package. The dependency on this kit is a
`file:` reference into the kit's checkout; a copy outside the kit depends on the
published package instead.
