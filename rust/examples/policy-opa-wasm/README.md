<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2026 Cognitio S.r.l.
-->

# policy-opa-wasm

A reference governance plugin, attached at the `ir-verdict` role, that evaluates
a Rego policy with the full
[OPA](https://www.openpolicyagent.org/) engine — the complete builtin surface and
exact reference-engine semantics — by driving an OPA-compiled WebAssembly module
inside the guest.

This is the full-fidelity road for Rego policies. The low-friction road is the
sibling [`policy-regorus`](../policy-regorus) guest, which embeds a Wasm-native
Rego evaluator and takes a `.rego` file at attach time. That evaluator
reimplements Rego independently and does not cover every builtin; when a policy
needs the complete OPA builtin surface or exact OPA semantics, use this adapter.

## What the road costs, and when to take it

The full-fidelity road buys the complete builtin surface and the reference
engine's own semantics. It is paid for in three things, and the low-friction road
pays none of them:

- **Artifact size.** The guest carries an embedded WebAssembly interpreter and the
  compiled policy module inside itself. Both are absent from a guest that embeds a
  Wasm-native evaluator and reads a policy file at attach time. The release
  profile in `Cargo.toml` optimises for size for exactly this reason, and
  `build.sh` runs a further size pass where the optimiser is available.
- **A build step, and a committed artifact.** The policy is compiled ahead of time
  with OPA's own toolchain and the result is committed beside the source. Editing
  `policy.rego` therefore means re-running that step and committing the new
  `policy.wasm`; it is not picked up by rebuilding the guest.
- **The policy is fixed at build time.** Because the module is baked in, an
  operator attaches settings and not a policy — there are no `sources`. A
  deployment that wants operators to supply their own Rego wants the low-friction
  road, or one adapter build per policy.

Reach for it when a policy needs a builtin the low-friction evaluator does not
implement, or when a decision has to match the reference engine exactly. Stay on
the low-friction road otherwise: it is smaller, it has no ahead-of-time compile,
and it lets an operator author the policy.

## How it works

The policy is authored in Rego and compiled **once, ahead of time** with OPA's
own toolchain into a WebAssembly module (`policy.wasm`), which is committed
alongside this source. The adapter embeds that module with `include_bytes!` and,
on every build:

1. Instantiates the baked OPA module inside the guest with an embedded pure-Rust
   WebAssembly interpreter (`wasmi`).
2. Satisfies the OPA module's own imports — its linear memory and its builtin /
   abort callbacks — with in-guest host functions. The OPA Wasm ABI is absorbed
   entirely here: the compiler host sees only the standard `describe()` /
   `validate()` verdict surface and never learns OPA is involved.
3. Runs OPA's evaluation sequence over every compiled entrypoint and maps each
   entrypoint's decision back to the socket's verdict shape.

Because the module is pre-baked, evaluation is self-contained: no OPA process
runs, nothing is fetched, and the build stays deterministic and hermetic.

## What it exports

- `describe()` — reports itself as a `governance` plugin, ABI `v1alpha1`,
  serving the `ir-verdict` role, exporting `validate`.
- `validate(input)` — drives the baked module against the payload the host hands
  it and returns the fired verdicts.

## Writing the policy

The baked policy declares its decision rules in a Rego package and compiles each
of `deny`, `warn`, and `audit` as an OPA entrypoint. Those names are Rego's own
naming for a decision rule; the adapter maps an entrypoint's trailing name onto
one of the socket's three verdict severities:

| Entrypoint | Verdict severity | Effect on the build                                      |
| ---------- | ---------------- | -------------------------------------------------------- |
| `deny`     | `halt`           | Each message stops the build.                            |
| `warn`     | `warn`           | Each message flags the build; the build proceeds.        |
| `audit`    | `info`           | Each message is recorded; the build outcome is unchanged. |

The payload is wired into OPA as follows:

- The whole payload is the OPA `input` document. A policy reaches the IR graph as
  `input.graph` and the routing paths as `input.precalculated_paths`.
- The attachment's `settings` object is the OPA `data` document. A policy reads
  an operator-supplied tunable as `data.<key>`.

The bundled [`policy.rego`](policy.rego) is a worked example: it denies when the
build graph carries more nodes than `data.max_nodes` permits, and warns when the
graph declares routing edges.

## Attaching the policy

An operator attaches the adapter at the `ir-verdict` role in the governance
socket configuration. The policy is baked into the guest, so only settings are
supplied — there are no `sources`:

```yaml
plugins:
  governance:
    - uri: <resolvable-plugin-reference>
      role: ir-verdict
      with:
        settings:
          max_nodes: 50
```

## Loud failure

The baked module declares which external builtins it needs. This adapter
provides none in-guest, so a module compiled to need a host-provided builtin is
**refused up front** with a message naming the requirement, and every builtin
callback is an error-stub trap as a backstop. Either way, a policy that reaches
for an unprovided builtin fails the build loudly — it never passes silently. "The
adapter cannot evaluate this policy" is deliberately distinct from "the policy
allows the build".

The reference `policy.rego` uses only builtins OPA compiles into the module
(`count`, comparison), so it needs no host builtin. A policy that needs one (for
example `sprintf`, `regex.match`, or `crypto.*`) requires extending the adapter
to implement that builtin in-guest; author the baked policy against the
compiled-in builtin set to keep the adapter thin.

## Regenerating `policy.wasm`

`policy.wasm` is a committed, pinned artifact. Regenerate it after editing
`policy.rego` with the OPA CLI (an authoring step — the build does **not** require
`opa` on PATH):

```sh
opa build -t wasm \
  -e aggrega/policy/deny \
  -e aggrega/policy/warn \
  policy.rego -o bundle.tar.gz
tar xzf bundle.tar.gz -O /policy.wasm > policy.wasm
rm bundle.tar.gz
```

Pass one `-e <package>/<rule>` for each decision rule you want compiled as an
entrypoint; add `-e aggrega/policy/audit` if the policy declares an `audit` rule.

## Building the plugin

Build the guest with its own `build.sh`, which dispatches through the pinned
toolchain and targets `wasm32-wasip1`:

```sh
./build.sh
```

## Toolchain isolation

This example embeds a WebAssembly interpreter and is therefore a self-contained
workspace: it carries its own `[workspace]` table, its own `rust-toolchain.toml`
pinning the current stable toolchain, and its own `Cargo.lock`, and the parent
plugin workspace excludes its path.
