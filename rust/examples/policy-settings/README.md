# policy-settings (Rust fixture)

A Rust policy plugin that **types its attachment settings**. Its
`describe()` self-report carries a `settings_schema` — a JSON Schema the host
compiles at load and validates the operator's `with.settings` against before
dispatching `validate`.

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
plugin runs. A conforming attachment reaches `validate`, which reads the typed
setting and emits a single audit verdict echoing the value it read — proof the
setting flowed through as the declared type.

A plugin that declares no `settings_schema` keeps the free-form pass-through
behaviour: the host hands its settings through verbatim without validation.

## Build

Rust toolchain with the `wasm32-unknown-unknown` target required. Run from this
directory:

```sh
bash build.sh
```

Destination resolution:

1. `$AGGREGA_PDK_WASM_OUT` if set — explicit override (the umbrella's
   integration test driver uses this when staging into a temporary directory).
2. The umbrella's `aggrega-cli/internal/wasm/testdata/` directory when this PDK
   is checked out as a submodule of the umbrella.
3. The PDK's own `testdata/` directory for standalone checkouts.

## Source-level dependency

This fixture is a workspace member of `aggrega-pdk-rust` and consumes the PDK via
a path dependency. The umbrella checkout provides the sibling structure.
Standalone reuse by end users is gated on PDK publication.
