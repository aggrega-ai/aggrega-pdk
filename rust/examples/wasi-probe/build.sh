#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the wasi-probe fixture to a .wasm artifact targeting
# wasm32-wasip1 (NOT wasm32-unknown-unknown — this fixture is supposed
# to emit real WASI imports so the host's stubbed-subset firewall has
# something to exercise).
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override; the umbrella's
#      integration test driver may set this when staging into a
#      temporary directory).
#   2. The umbrella's testdata directory, when this PDK is checked
#      out as a submodule of the aggrega-umbrella superproject.
#   3. The PDK's own root testdata/, for genuine standalone checkouts.

set -eo pipefail

if [ -f "${HOME}/.cargo/env" ]; then
    # shellcheck disable=SC1091
    . "${HOME}/.cargo/env"
fi

# Ensure the wasm32-wasip1 target is installed (idempotent).
rustup target add wasm32-wasip1 >/dev/null 2>&1 || true

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

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKSPACE_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

(cd "${WORKSPACE_ROOT}" && cargo build --release --target wasm32-wasip1 -p wasi-probe)

cp "${WORKSPACE_ROOT}/target/wasm32-wasip1/release/wasi_probe.wasm" "${OUT_DIR}/wasi-probe-rust.wasm"
echo "Wrote ${OUT_DIR}/wasi-probe-rust.wasm"
