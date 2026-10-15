#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the importer scaffold plugin to a .wasm artifact.
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override; the build invokes it
#      this way from inside a temporary or staging directory).
#   2. Otherwise, the PDK's own root testdata/.

set -eo pipefail

# This example is a standalone module (its own go.mod + replace), not a
# member of any go.work in a parent directory. Disable the workspace so
# the build resolves dependencies through this module rather than failing
# the go.work membership check when a parent Go workspace encloses it.
export GOWORK=off

if [ -n "${AGGREGA_PDK_WASM_OUT:-}" ]; then
    OUT_DIR="${AGGREGA_PDK_WASM_OUT}"
else
    OUT_DIR="$(git rev-parse --show-toplevel)/testdata"
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
    -o "${OUT_DIR}/import-go.wasm" ./
echo "Wrote ${OUT_DIR}/import-go.wasm"
