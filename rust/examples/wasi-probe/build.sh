#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the wasi-probe fixture to a .wasm artifact targeting
# wasm32-wasip1 (NOT wasm32-unknown-unknown — this fixture is supposed
# to emit real WASI imports so the host's stubbed-subset firewall has
# something to exercise).
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override; the aggrega-cli test
#      harness sets this when it builds this example as a fixture,
#      staging into a temporary directory).
#   2. Otherwise, the PDK's own root testdata/.

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
    OUT_DIR="$(git rev-parse --show-toplevel)/testdata"
fi

mkdir -p "${OUT_DIR}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKSPACE_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

(cd "${WORKSPACE_ROOT}" && cargo build --release --target wasm32-wasip1 -p wasi-probe)

cp "${WORKSPACE_ROOT}/target/wasm32-wasip1/release/wasi_probe.wasm" "${OUT_DIR}/wasi-probe-rust.wasm"
echo "Wrote ${OUT_DIR}/wasi-probe-rust.wasm"
