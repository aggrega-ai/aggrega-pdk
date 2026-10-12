#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the agent-flow importer plugin to a .wasm artifact.
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override; the build invokes it
#      this way from inside a temporary or staging directory).
#   2. The umbrella's testdata directory, when this PDK is checked out
#      as a submodule of the aggrega-umbrella superproject.
#   3. The PDK's own root testdata/, for genuine standalone checkouts.

set -eo pipefail

# This example is a standalone module (its own go.mod + replace), not a
# member of the umbrella go.work. Disable the workspace so the build
# resolves dependencies through this module rather than failing the
# go.work membership check when invoked from inside the umbrella tree.
export GOWORK=off

if [ -n "${AGGREGA_PDK_WASM_OUT:-}" ]; then
    OUT_DIR="${AGGREGA_PDK_WASM_OUT}"
else
    SUPERPROJECT="$(git rev-parse --show-superproject-working-tree 2>/dev/null)"
    if [ -n "$SUPERPROJECT" ]; then
        OUT_DIR="${SUPERPROJECT}/aggrega-cli/internal/wasm/testdata"
    else
        OUT_DIR="$(git rev-parse --show-toplevel)/testdata"
    fi
fi

# Version the importer self-reports through describe(). The linker folds this
# into the descriptor's Metadata, so the running guest announces its own version
# at load, and a Plugin entity is generated from the built guest at that version
# — `aggrega plugin gen-catalog` refuses a guest that reports none. The publish
# door cross-checks the self-reported version against a Plugin entity's
# metadata.version and halts on a mismatch, so the stamp keeps the guest and its
# entity in lockstep. Commit is deliberately left unstamped: the guard checks
# only version, and a dynamic commit hash would make the artifact
# non-deterministic across builds.
PLUGIN_VERSION="${AGGREGA_PLUGIN_VERSION:-1.0.0}"

mkdir -p "${OUT_DIR}"
# Built with the standard Go toolchain targeting GOOS=wasip1. The guest is a
# WASI reactor whose runtime bootstraps under the host's deterministic,
# capability-denied WASI configuration; -buildmode=c-shared produces a reactor
# (no main entry, the //go:wasmexport functions are the exports). The standard
# toolchain's full GC and runtime decode the import payload through the generated
# ABI codec without the collector caveats the bare-Wasm targets carry.
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared \
    -ldflags "-X github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega.Version=${PLUGIN_VERSION}" \
    -o "${OUT_DIR}/import-langflow-go.wasm" ./
echo "Wrote ${OUT_DIR}/import-langflow-go.wasm"
