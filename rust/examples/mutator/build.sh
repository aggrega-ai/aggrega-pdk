#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the mutator fixture plugin to a .wasm artifact.
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override; the aggrega-cli test
#      harness sets this when it builds this example as a fixture,
#      staging into a temporary directory).
#   2. Otherwise, the PDK's own root testdata/.

set -eo pipefail

# Bring cargo onto PATH for non-interactive subprocess invocations.
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

# Build the cdylib via Cargo. The workspace's target/ directory holds
# the output; the crate name is `mutator-rust`, Cargo emits
# `mutator_rust.wasm` (underscore form per Cargo's library naming
# convention). Copy with the dash form the integration test expects.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKSPACE_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

(cd "${WORKSPACE_ROOT}" && cargo build --release --target wasm32-unknown-unknown -p mutator-rust)

cp "${WORKSPACE_ROOT}/target/wasm32-unknown-unknown/release/mutator_rust.wasm" "${OUT_DIR}/mutator-rust.wasm"
echo "Wrote ${OUT_DIR}/mutator-rust.wasm"
