#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the policy-validator fixture plugin to a .wasm artifact.
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

mkdir -p "${OUT_DIR}"
tinygo build -target=wasm-unknown -o "${OUT_DIR}/policy-validator-go.wasm" ./
echo "Wrote ${OUT_DIR}/policy-validator-go.wasm"
