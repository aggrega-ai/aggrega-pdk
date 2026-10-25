# capability-label (Go)

A Go plugin for the label-transform role that adds a capability label when a
developer's configuration earns it. It reads each node's declared `values`
from the `MutatorInput` payload, and a node that is a datastore, carries no
`datastore.aggrega.ai/engine` label of its own, and declares

```yaml
values:
  extensions: [vector]
```

receives `datastore.aggrega.ai/engine: vector`. Every other node is left as it
is, so a datastore whose values enable no vector extension stays unlabelled on
that dimension.

A capability that depends on configuration cannot sit on the catalog
Component, because the catalog cannot know how each instance is configured. The
values the developer declares are what decide it, and the label-transform role
is where they are read: the plugin receives every node's values, read-only,
and returns JSON Patch documents the host applies to the node labels alone. A
socket or a Feature that selects `datastore.aggrega.ai/engine=vector` then
matches the promoted node, because selectors read the labels the transform
left.

The values a node carries are its instance's values as the Aggregate declares
them, with the Aggregate's templates evaluated against its parameters. A
default the Component's schema declares, and anything a Recipe or a Feature
contributes, are not among them, so a configuration the rule reads has to be
written on the instance. A value the Component declares sensitive arrives
withheld, as the single-key `aggrega_withheld` object, so no rule can read it.

## Attach it

Attach the built plugin at the `label-transform` role under
`plugins.governance` in the workspace configuration, as any governance plugin
is attached.

## Build

Standard Go required (the build targets `GOOS=wasip1`). Run from this
directory:

```sh
bash build.sh
```

Destination resolution:

1. `$AGGREGA_PDK_WASM_OUT` if set — explicit override.
2. Otherwise, the PDK's own `testdata/` directory.

## Source-level dependency

This example consumes the PDK through a `replace` directive in `go.mod`
pointing at the sibling PDK source at `../../`, and the ABI module the same
way, from the `aggrega-abi` repository checked out beside this repository.
