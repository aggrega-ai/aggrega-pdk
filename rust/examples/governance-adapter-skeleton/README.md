<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2026 Cognitio S.r.l.
-->

# governance-adapter-skeleton

A skeleton for a governance plugin in Rust. Copy this directory, drop your policy
engine into the seam, and the result is a guest an operator can attach at every
governance role.

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
| `ir-verdict` | `verdict_ir` | the build graph, plus the routing paths enumerated alongside it | verdicts |
| `render-verdict` | `verdict_render` | the rendered output files, each with its text and, where the host parsed it, a structured view of its content | verdicts |

Each verdict role has its own export, so the export you name is what says which
role the guest is serving and which payload it is reading. The two verdict
exports share one implementation, because both verdict payloads carry the same
two things the engine needs: the attachment's programs and its settings.

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
  needs. The settings object arrives verbatim too, and is `None` when the
  attachment declares none.
- **the input is the payload of the attached role** — the four in the table
  above.
- **the output is either the verdicts the engine reached, or the changes a
  transform role asks for.**

## The seam

`evaluate` in [`src/evaluator.rs`](src/evaluator.rs) is the one module a new
adapter has to rewrite:

```rust
pub fn evaluate(
    programs: &[Program<'_>],
    settings: Option<&str>,
    payload: &str,
    decision: &str,
) -> anyhow::Result<Vec<Box<RawValue>>>
```

It receives the programs, the settings, the whole role payload, and the name of
the decision the caller wants back. It returns one raw JSON value per decision.

The payload reaches the seam as a JSON document at every role. The
`label-transform` role's wire carries canonical CBOR, so that role reads the
payload with the generated codec and writes it back out as JSON before handing it
over — and its export takes and returns `Vec<u8>` rather than `String`; the other
three roles carry JSON and hand over what arrived. The seam therefore has one
contract rather than one per encoding, and the names a policy addresses are the
payload's own names either way.

Handing over the whole payload rather than a projection of it is deliberate: a
policy then reads the graph, the artifacts, the nodes or the targets under the
names the wire already gives them, so the input a policy author writes against is
learnable from one place.

Two obligations on whatever replaces the body:

- **Identical input must yield an identical sequence of decisions.** A build's
  output is byte-identical across runs of identical input, and an adapter that
  iterated an unordered collection would break that from inside the guest. Where
  an engine yields a set, sort it before returning.
- **A failure to evaluate must return an error, never an empty result.** An empty
  result is a policy that found nothing to say; an error is a policy that did not
  run. Reporting the second as the first lets a build through unexamined.

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
unchanged. `COLLECTIONS` in [`src/verdicts.rs`](src/verdicts.rs) is the mapping
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
`src/labels.rs` builds the path. The escaping is written once rather than in
every policy, and the module's own tests cover both the separator and the escape
character.

## Layout, and what to delete

```text
governance-adapter-skeleton/
├── README.md            (this file)
├── Cargo.toml
├── build.sh             (build to dist/)
└── src/
    ├── lib.rs           (the describe() self-report and the exports)
    ├── evaluator.rs     (THE SEAM — the one module a new adapter rewrites)
    ├── annotations.rs   (the annotation-transform role)
    ├── labels.rs        (the label-transform role)
    └── verdicts.rs      (the ir-verdict and render-verdict roles)
```

One module per role, so the unit of deletion is the role. An adapter that only
renders verdicts deletes `annotations.rs` and `labels.rs`, their `mod`
declarations, their exports, and the two matching entries in `descriptor()`. The
host refuses to call an export the guest did not declare, so an attachment
pointed at a role the adapter does not serve is refused by name rather than
silently doing nothing.

## Wire types come from the generated codec

Every payload here is a generated type, generated from the same schemas the host
reads, so the guest and the host name one shape and neither can drift from the
other. Some are re-exported by this kit (`aggrega_pdk_rust::PolicyInput`,
`aggrega_pdk_rust::SEVERITY_HALT`) and some are reached through the codec crate
directly (`aggrega_abi::AnnotationTransformInput`,
`aggrega_abi::RenderPolicyInput`); both spellings name the identical type, and
the crate is a dependency for exactly that reason. Do not hand-write a struct for
a payload — a hand-written one is a second copy of the shape, and the copy is what
goes out of step.

## Build

```sh
./build.sh
```

The artifact lands at `dist/governance-adapter-skeleton-rust.wasm`. Set
`AGGREGA_PDK_WASM_OUT` to stage it elsewhere.

This crate is an ordinary member of the enclosing plugin workspace: it has no
engine dependency of its own, so it resolves through the shared lockfile. An
adapter whose engine needs a newer toolchain or a divergent dependency tree
becomes a self-contained workspace instead — its own `[workspace]` table, its own
`rust-toolchain.toml` and its own `Cargo.lock`, with the parent workspace
excluding its path. [`../policy-opa-wasm`](../policy-opa-wasm) is that shape.

## Check it runs, before attaching it

`aggrega plugin run-local` loads a guest from a path, checks the export against
the guest's own `describe()` self-report, calls it, and writes the response to
standard output. One call per role:

```sh
aggrega plugin run-local --plugin dist/governance-adapter-skeleton-rust.wasm \
  --type governance --export transform_annotations --with @annotation-payload.json

aggrega plugin run-local --plugin dist/governance-adapter-skeleton-rust.wasm \
  --type governance --export mutate --with @label-payload.json

aggrega plugin run-local --plugin dist/governance-adapter-skeleton-rust.wasm \
  --type governance --export verdict_ir --with @ir-verdict-payload.json

aggrega plugin run-local --plugin dist/governance-adapter-skeleton-rust.wasm \
  --type governance --export verdict_render --with @render-verdict-payload.json
```

The response reaches you exactly as the guest emitted it, and the wire an export
carries is fixed by the socket that export serves rather than by any flag, so
these calls legitimately differ in what they print: the `mutate` call prints
bytes, and the rest print readable text.

An unmodified skeleton reaches no engine, so each role emits its own neutral
result — no annotations, no label changes, and one `info` note per verdict role
naming the programs that reached the seam. That note is the wiring proof: the
guest decoded the payload the host handed it, ran, and emitted the shape its role
is allowed to emit. It comes from the placeholder in `src/evaluator.rs` and goes
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
file with your own, and rename the crate. The `path` dependencies point at this
kit's checkout; a copy outside the kit depends on the published crates instead.
