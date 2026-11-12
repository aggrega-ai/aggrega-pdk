<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2026 Cognitio s.r.l.
-->

# policy-kubewarden-render

A reference governance plugin, attached at the `render-verdict` role, that runs a
[Kubewarden](https://kubewarden.io/)-style validating policy over the rendered
Kubernetes manifests. It is an *adapter*: it speaks the Aggrega plugin ABI
outward and absorbs the Kubewarden admission-control contract inward, so the
compiler stays free of any Kubewarden or admission-control logic. Every policy
decision is made inside the guest; the host only routes the rendered artifacts
in and the verdicts out.

## Why an adapter

Kubewarden policies are written against the Kubernetes
[AdmissionReview](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)
contract: a policy receives a `ValidationRequest` that wraps a single admission
request object and returns a `ValidationResponse` that accepts or rejects it.

At the render stage the compiler's output already *is* a set of Kubernetes
resources. That makes the render stage the natural place to run an
admission-shaped policy: this adapter wraps each rendered object in an
AdmissionReview-shaped request, runs the Kubewarden policy against it, and maps
the response back to an Aggrega verdict.

```text
rendered manifests ──▶ adapter (our ABI)
                          │  per object: wrap as ValidationRequest
                          ▼
                       Kubewarden policy  (accept / reject)
                          │  map response
                          ▼
                       Aggrega verdicts ──▶ host (halt stops, warn/info record)
```

The `ValidationRequest` / `ValidationResponse` wire types are local structs
shaped **exactly** per the published
[`kubewarden-policy-sdk`](https://crates.io/crates/kubewarden-policy-sdk)
v0.17.3 (`src/request.rs`, `src/response.rs`) — the same field names, the same
JSON representation, the same admission-request layout — so the in-guest policy
sees the same `ValidationRequest` a real Kubewarden host would deliver. They are
reproduced locally rather than pulled from the SDK crate because the SDK depends
on `wapc-guest`, whose unconditional `#[no_mangle] __guest_call` export forces a
`wapc` host-module import — the waPC guest ABI — that this runtime's plugin
loader does not host. The guest's sole export is its own Aggrega `verdict_render`; the
Kubewarden policy is called as an ordinary in-guest Rust function,
`evaluate(ValidationRequest) -> ValidationResponse`.

## What it does

The plugin exports the two functions a verdict-role governance plugin declares:

- `describe()` — reports itself as a `governance` plugin at the kit's ABI,
  serving the `render-verdict` role, exporting `verdict_render`.
- `verdict_render(input)` — receives the render-stage payload (the rendered
  artifacts, plus the attachment's sources and settings), runs the policy over
  each rendered Kubernetes object, and returns the fired verdicts.

For each artifact — a rendered manifest, possibly a multi-document YAML file —
the adapter parses every document into an object. For each object it derives the
admission fields *from the parsed object itself* (kind and API group/version
from `apiVersion`/`kind`, name and namespace from `metadata`; operation
`CREATE`), synthesizes a `ValidationRequest`, and runs the policy. A rejection
becomes one verdict; an acceptance emits nothing. Documents that are not
Kubernetes objects (empty documents, or mappings without a `kind`) are skipped.

## The reference policy

The bundled policy is a **require-label** rule, a common Kubewarden pattern:
every object must carry a specific label. The label key is named by the
attachment's `settings.required_label`. An object missing the label is
rejected with a clear message; an object carrying it is accepted. An empty or
absent `required_label` imposes no requirement.

## Attaching the policy

An operator attaches the plugin at the `render-verdict` role in the governance
socket configuration, supplying settings:

```yaml
plugins:
  governance:
    - uri: <plugin reference>
      role: render-verdict
      with:
        settings:
          required_label: app.kubernetes.io/name
```

## Settings

| key              | meaning                                                              |
| ---------------- | -------------------------------------------------------------------- |
| `required_label` | the label key every rendered object must carry (the policy setting)  |
| `severity`       | how a rejection maps to a verdict: `halt` (default), `warn`, `info` |

`required_label` is the Kubewarden policy's own setting. `severity` is the
adapter's mapping knob: a rejection maps to a `halt` verdict by default, which
halts the build; `warn` flags the build and lets it proceed; `info` records
without affecting the outcome. Any other value maps to `halt`.

## Determinism

The host hands the artifacts already sorted by path. The guest walks them in
that order, walks each artifact's documents in document order, and emits
verdicts in that order — no map iteration enters the emit path — so identical
input yields a byte-identical verdict sequence.

## Validating only

The adapter is a validating policy: it accepts or rejects, and never returns a
mutated object. A Kubewarden mutating policy's `mutated_object` is ignored — the
compiler never rewrites rendered output from a policy verdict.

## Scope

This adapter covers **render-stage, Kubernetes output only**. Explicitly out of
scope:

- The IR stage. This adapter reasons over rendered manifests, not the build
  graph; a verdict over the build graph is the `ir-verdict` role.
- Non-Kubernetes render output (for example compose, Terraform, or
  CloudFormation artifacts). Render-stage adapters for other output shapes are
  possible follow-ons.
- Loading a compiled Kubewarden policy `.wasm` and driving its native waPC
  entrypoints. This adapter bundles the policy as in-guest Rust; a full-fidelity
  path that loads an external compiled Kubewarden policy through the attachment's
  `with.sources` is a possible follow-on.

## Building

```sh
./build.sh
```

The crate is a self-contained workspace with its own toolchain pin and
lockfile — so its dependency tree (including the YAML parser it pulls in)
resolves independently of the enclosing plugin workspace's shared,
deliberately-floored lockfile — so `build.sh` builds it under
`rustup run stable` and copies the artifact to `$AGGREGA_PDK_WASM_OUT` when
set, and otherwise to the PDK's own `testdata/` directory.
