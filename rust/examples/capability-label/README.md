# capability-label (Rust)

A Rust plugin for the label-transform role that adds a capability label when a
developer's configuration earns it — the Rust counterpart of the Go
`capability-label` example, with the same rule. It reads each node's declared
`values` from the `MutatorInput` payload, and a node that is a datastore,
carries no `datastore.aggrega.ai/engine` label of its own, and declares

```yaml
values:
  extensions: [vector]
```

receives `datastore.aggrega.ai/engine: vector`. Every other node is left as it
is.

Each value arrives as the verbatim JSON of the value the developer declared,
held in the payload's order-keeping map in the order the Aggregate wrote the
keys, so the plugin parses only the values its rule reads. The values are
read-only: the plugin returns JSON Patch documents, and the host applies them
to the node labels alone.

The values a node carries are its instance's values as the Aggregate declares
them, with the Aggregate's templates evaluated against its parameters. A
default the Component's schema declares, and anything a Recipe or a Feature
contributes, are not among them, and a value the Component declares sensitive
arrives withheld, as the single-key `aggrega_withheld` object.

## Build

Requires the `wasm32-unknown-unknown` target. Run from this directory:

```sh
bash build.sh
```

Destination resolution:

1. `$AGGREGA_PDK_WASM_OUT` if set — explicit override.
2. Otherwise, the PDK's own `testdata/` directory.
