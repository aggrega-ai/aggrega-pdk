<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2026 Cognitio S.r.l.
-->

# policy-regorus

A reference governance plugin, attached at the `ir-verdict` role, that evaluates
operator-supplied
[Rego](https://www.openpolicyagent.org/docs/latest/policy-language/) policies
against the build's IR graph. The Rego engine is embedded in the plugin, so
the compiler runs no policy engine of its own and carries no policy-language
logic: every policy decision is made inside the guest, and the host only
routes the payload in and the verdicts out.

## What it does

The plugin exports the two functions a verdict-role governance plugin declares:

- `describe()` — reports itself as a `governance` plugin, ABI
  `v1alpha1`, serving the `ir-verdict` role, exporting `validate`.
- `validate(input)` — evaluates the attached policies against the payload the
  host hands it and returns the fired verdicts.

On every build the governance socket hands `validate` a JSON payload with the IR graph,
the precalculated routing paths, the policy source files the attachment named,
and the attachment's free-form settings object.

## Attaching a policy

An operator attaches the plugin at the `ir-verdict` role in the governance
socket configuration, naming the `.rego` files as sources and any tunables as
settings:

```yaml
plugins:
  governance:
    - uri: <resolvable-plugin-reference>
      role: ir-verdict
      with:
        sources:
          - ./policies/replicas.rego
        settings:
          max_replicas: 3
```

The host reads each source file at compile time and passes its bytes to the
plugin verbatim; it never parses the file. The settings object is passed
through untouched.

## Writing a policy

A policy declares its decision rules in a Rego package and exposes up to three
collections of message strings:

| Collection | Effect on the build                                   |
| ---------- | ----------------------------------------------------- |
| `deny`     | Each message halts the build.                         |
| `warn`     | Each message flags the build; the build proceeds.     |
| `audit`    | Each message is recorded; the build outcome is unchanged. |

Each collection is a set or array of strings. A rule that stays undefined
contributes nothing, so a policy that never populates `deny` allows the build.
Every package the attached sources declare is evaluated, so a policy may split
its rules across packages.

The payload is wired into Rego as follows:

- The whole payload is the Rego `input` document. A policy reaches the IR
  graph as `input.graph` and the routing paths as `input.precalculated_paths`.
- The attachment's `settings` object is the Rego `data` document. A policy
  reads an operator-supplied tunable as `data.<key>` — for the example above,
  `data.max_replicas`.

A minimal deny policy:

```rego
package aggrega.policy

import rego.v1

deny contains msg if {
    some node in input.graph.spec.nodes
    count(node.value.replicas) > data.max_replicas
    msg := sprintf("node %v exceeds the replica ceiling", [node.key])
}
```

## Which rule syntax a policy is written in

Rego is written in two rule syntaxes and this engine accepts both, module by
module, so a policy set assembled before the current syntax existed evaluates
alongside one written today. A module in the current syntax says which one it is
by declaring `import rego.v1` below its package declaration, as the policy above
does; a module in the older bracketed-head syntax — `deny[msg] { ... }` —
declares nothing, because it is recognised by not parsing as the current one.

This is a reference example rather than a production engine, and that is where
the difference bites. A module that parses as the current syntax while declaring
nothing is read here in the older syntax, and the two readings can reach opposite
decisions from the same text: a rule body that never holds contributes no
decision under the current syntax and contributes one under the older syntax.
This example evaluates such a module rather than refusing it. A guest attached to
a real build should refuse it instead, naming the module and asking for the
declaration — the Rego engine that ships in the compiler does exactly that, and
the engine itself is what tells the two syntaxes apart, so a deliberate
bracketed-head policy is never mistaken for a current-syntax module that omitted
the declaration. When
this example is the starting point for a guest of your own, that refusal is the
part to add.

## Builtin coverage and loud failure

The embedded engine implements most of the Rego builtin library, but its
coverage lags the reference OPA implementation: some builtins are not yet
available in-engine. When a policy calls a builtin the engine does not
implement, evaluating the rule fails.

The plugin treats that failure as a fault, not a verdict: it returns an error
rather than an empty or allow result. The build then halts with a fault naming
the plugin, which is deliberately distinct from a deny verdict. "The engine
cannot evaluate this policy" must never be mistaken for "the policy allows the
build" — an unevaluable policy stops the build loudly instead of passing
silently.

If a policy needs a builtin the embedded engine lacks, use the full-fidelity
escape hatch below rather than working around the gap.

## Escape hatch: full-fidelity OPA via `opa build -t wasm`

When a policy needs the complete OPA builtin surface or exact OPA evaluation
semantics, compile the policy with OPA's own toolchain and run the result
behind a thin adapter plugin:

1. Compile the policy to a WebAssembly module with the OPA CLI:

   ```sh
   opa build -t wasm -e <package>/deny <package>/warn <package>/audit policy.rego
   ```

   This produces a bundle containing a `policy.wasm` built by OPA, which
   implements OPA's own Wasm ABI (its own entrypoints, memory model, and
   builtin-callback protocol).

2. Wrap that module in a governance adapter plugin attached at a verdict role.
   The adapter absorbs OPA's Wasm ABI entirely inside the guest: it instantiates
   the OPA-built module, sets the input, invokes the evaluation entrypoint, and
   maps the OPA decision back to the `deny` / `warn` / `audit` verdict shape
   the socket expects. The compiler host sees only the standard
   `describe()` / `validate()` verdict surface; it never learns that
   OPA is involved and carries no OPA-specific logic.

The adapter keeps the same host contract as this plugin, so switching a policy
from the embedded engine to the full OPA engine is an attachment change, not a
host change. Building that adapter is a separate, heavier undertaking than this
embedded-engine plugin; this README documents the path rather than shipping the
adapter.

## Toolchain isolation

This example embeds an evaluator whose dependency requires a newer Rust than
the enclosing plugin workspace floors itself to. It is therefore a
self-contained workspace: it carries its own `[workspace]` table, its own
`rust-toolchain.toml` pinning the current stable toolchain, and its own
`Cargo.lock`, and the parent workspace excludes its path. Build it with its own
`build.sh`, which dispatches through the pinned toolchain and targets
`wasm32-wasip1`.
