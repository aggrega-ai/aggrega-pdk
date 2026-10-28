# policy-settings (Rust fixture)

A Rust governance plugin for the governance socket's `ir-verdict` role that
**types its attachment settings**. Its
`describe()` self-report carries a `settings_schema` — a JSON Schema the host
compiles at load and validates the operator's `with.settings` against before
dispatching `verdict_ir`.

The declared schema requires an integer `max_replicas` of at least 1 and
forbids unknown keys:

```json
{
  "type": "object",
  "properties": { "max_replicas": { "type": "integer", "minimum": 1 } },
  "required": ["max_replicas"],
  "additionalProperties": false
}
```

An attachment whose settings omit `max_replicas`, give it the wrong type, or add
an unknown key halts the build loudly (naming the offending key) before the
plugin runs. A conforming attachment reaches `verdict_ir`, which reads the typed
setting and emits a single verdict of severity `info` with the message
`typed settings applied: max_replicas=<value>`, echoing the value it read —
proof the setting flowed through as the declared type.

A plugin that declares no `settings_schema` keeps the free-form pass-through
behaviour: the host hands its settings through verbatim without validation.

## Build

Rust toolchain with the `wasm32-unknown-unknown` target required. Run from this
directory:

```sh
bash build.sh
```

Destination resolution:

1. `$AGGREGA_PDK_WASM_OUT` if set — explicit override (the `aggrega-cli` test
   harness sets it when it builds this example as a fixture, staging into a
   temporary directory).
2. Otherwise, the PDK's own `testdata/` directory.

## Source-level dependency

This fixture is a workspace member of `aggrega-pdk-rust` and consumes the PDK via
a path dependency. The PDK in turn takes the ABI crate by path from the
`aggrega-abi` repository checked out beside this repository. Standalone reuse by
end users is gated on PDK publication.
