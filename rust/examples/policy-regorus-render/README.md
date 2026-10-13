<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2026 Cognitio S.r.l.
-->

# policy-regorus-render

A reference governance plugin, attached at the `render-verdict` role, that
evaluates operator-supplied
[Rego](https://www.openpolicyagent.org/docs/latest/policy-language/) policies
against the compiler's **rendered output artifacts**. It is the render-stage
sibling of the `policy-regorus` plugin, which evaluates Rego against the build's
IR graph at the `ir-verdict` role. The Rego engine is embedded in the guest, so the compiler runs no
policy engine of its own and carries no policy-language or output-format logic.

## Output-shape independence

The adapter is not tied to Kubernetes or to any one render target. It parses
each rendered artifact's content as one or more YAML or JSON documents — YAML is
a superset of JSON, so a JSON template (for example a CloudFormation template)
parses through the same parser — and exposes the parsed documents to the policy
alongside the raw content. A policy reasons over the structured render output of
any YAML/JSON target with the same Rego it would write against any structured
document: Docker Compose services, CloudFormation resources, and so on.

### Native-HCL Terraform output

A native Terraform file (a bare `.tf`) is neither YAML nor JSON, so it is
projected through an HCL reader into the same canonical object a `.tf.json` file
parses to:

```text
input.artifacts[_].documents[_].resource.<type>.<name>.<attr>
```

A policy reads a Terraform resource attribute the same way whichever form the
compiler emitted — for an RDS instance, for example,
`input.artifacts[_].documents[_].resource.aws_db_instance[_].allocated_storage`.
Every other artifact — `.tf.json`, `.auto.tfvars.json`, and Compose or
CloudFormation YAML/JSON — stays on the YAML/JSON path unchanged. A `.tftpl` is
Terraform template text rather than standalone HCL, so it is not projected and,
like any non-structured content, surfaces `parse_error`.

## What it does

The plugin exports the two functions a verdict-role governance plugin declares:

- `describe()` — reports itself as a `governance` plugin, ABI
  `v1alpha1`, serving the `render-verdict` role, exporting `verdict_render`.
- `verdict_render(input)` — evaluates the attached policies against the rendered
  artifacts the host hands it and returns the fired verdicts.

At the render stage the governance socket hands `verdict_render` a payload in
canonical CBOR with the rendered artifacts (each an output path and its content),
the policy source files the attachment named, and the attachment's free-form
settings object. The plugin reads it with `RenderPolicyInput::decode_cbor` and
writes its verdicts with `PolicyOutput::encode_cbor`.

## Attaching a policy

An operator attaches the plugin at the `render-verdict` role in the governance
socket configuration, naming the `.rego` files as sources and any tunables as
settings:

```yaml
plugins:
  governance:
    - uri: <resolvable-plugin-reference>
      role: render-verdict
      with:
        sources:
          - ./policies/registry.rego
        settings:
          approved_registry: registry.example.com
```

The host reads each source file at compile time and passes its bytes to the
plugin verbatim; it never parses the file. The settings object is passed
through untouched.

## Writing a policy

A policy declares its decision rules in a Rego package and exposes up to three
collections of message strings:

| Collection | Effect on the build                                       |
| ---------- | -------------------------------------------------------- |
| `deny`     | Each message halts the build.                            |
| `warn`     | Each message flags the build; the build proceeds.        |
| `audit`    | Each message is recorded; the build outcome is unchanged.|

Each collection is a set or array of strings. A rule that stays undefined
contributes nothing, so a policy that never populates `deny` allows the build.
Every package the attached sources declare is evaluated, so a policy may split
its rules across packages.

The Rego `input` document is the render payload enriched with the parsed
documents:

- `input.artifacts[_].path` — the artifact's output path.
- `input.artifacts[_].content` — the raw rendered text, verbatim.
- `input.artifacts[_].documents` — the parsed documents at that path, in
  document order: YAML/JSON documents for a YAML/JSON artifact, or the single
  projected object for a native `.tf`.
- `input.artifacts[_].parse_error` — present only when the content did not parse;
  carries the parser's message.
- `input.sources`, `input.settings` — the attachment's own sources and settings.

The attachment's `settings` object is also the Rego `data` document, so a policy
reads an operator-supplied tunable as `data.<key>`.

A minimal deny policy over Docker Compose output — every service image must come
from an approved registry (carry a registry-prefix `/`):

```rego
package aggrega.render

import rego.v1

deny contains msg if {
    some artifact in input.artifacts
    some document in artifact.documents
    some name, service in document.services
    not contains(service.image, "/")
    msg := sprintf("service %v image %q is not from an approved registry", [name, service.image])
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

## Parse failures do not silently allow

When an artifact's content does not parse as YAML/JSON, the adapter neither
crashes nor drops the artifact: it keeps the raw `content`, sets `documents` to
an empty array, and records the parser's message under `parse_error`. A policy
that reasons over structured documents sees no documents for that artifact, but
the malformed artifact stays visible — a policy can `deny` on
`input.artifacts[_].parse_error` to make a malformed render a hard failure. The
adapter never decides an unparseable artifact is compliant on the policy's
behalf.

## Builtin coverage and loud failure

The embedded engine implements most of the Rego builtin library, but its
coverage lags the reference OPA implementation. When a policy calls a builtin
the engine does not implement, evaluating the rule fails, and the plugin treats
that failure as a fault, not a verdict: it returns an error rather than an empty
or allow result. The build then halts with a fault naming the plugin, which is
deliberately distinct from a deny verdict. "The engine cannot evaluate this
policy" must never be mistaken for "the policy allows the build". For policies
that need the complete OPA builtin surface, compile the policy with OPA's own
`opa build -t wasm` toolchain and wrap the result behind a thin adapter plugin,
as described in the `policy-regorus` README.

## Toolchain isolation

This example embeds an evaluator whose dependency requires a newer Rust than the
enclosing plugin workspace floors itself to, and pulls a YAML parser that must
not enter the shared lockfile. It is therefore a self-contained workspace: it
carries its own `[workspace]` table, its own `rust-toolchain.toml` pinning the
current stable toolchain, and its own `Cargo.lock`, and the parent workspace
excludes its path. Build it with its own `build.sh`, which dispatches through the
pinned toolchain and targets `wasm32-wasip1`.
