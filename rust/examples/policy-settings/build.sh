#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the policy-settings fixture plugin to a .wasm artifact.
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

if [ -n "${AGGREGA_PDK_WASM_OUT:-}" ]; then
    OUT_DIR="${AGGREGA_PDK_WASM_OUT}"
else
    OUT_DIR="$(git rev-parse --show-toplevel)/testdata"
fi

mkdir -p "${OUT_DIR}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKSPACE_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

(cd "${WORKSPACE_ROOT}" && cargo build --release --target wasm32-unknown-unknown -p policy-settings-rust)

cp "${WORKSPACE_ROOT}/target/wasm32-unknown-unknown/release/policy_settings_rust.wasm" "${OUT_DIR}/policy-settings-rust.wasm"
echo "Wrote ${OUT_DIR}/policy-settings-rust.wasm"
