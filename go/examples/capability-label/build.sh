#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the capability-label plugin to a .wasm artifact.
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override, for a build staged into a
#      temporary directory).
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

# Version the guest self-reports through describe(); a Plugin entity's
# metadata.version is cross-checked against it when the plugin is published.
PLUGIN_VERSION="${AGGREGA_PLUGIN_VERSION:-1.0.0}"

mkdir -p "${OUT_DIR}"
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared \
    -ldflags "-X github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega.Version=${PLUGIN_VERSION}" \
    -o "${OUT_DIR}/capability-label-go.wasm" ./
echo "Wrote ${OUT_DIR}/capability-label-go.wasm"
