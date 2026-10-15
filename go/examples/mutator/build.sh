#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the mutator fixture plugin to a .wasm artifact.
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

mkdir -p "${OUT_DIR}"
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o "${OUT_DIR}/mutator-go.wasm" ./
echo "Wrote ${OUT_DIR}/mutator-go.wasm"
