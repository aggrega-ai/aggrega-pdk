#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the search fixture plugin to a .wasm artifact.
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override; the umbrella's
#      integration test driver may set this when staging into a
#      temporary directory).
#   2. The umbrella's testdata directory, when this PDK is checked
#      out as a submodule of the aggrega-umbrella superproject.
#   3. The PDK's own root testdata/, for genuine standalone checkouts.

set -eo pipefail

# Bring cargo onto PATH for non-interactive subprocess invocations.
if [ -f "${HOME}/.cargo/env" ]; then
    # shellcheck disable=SC1091
    . "${HOME}/.cargo/env"
fi

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

# Build the cdylib via Cargo. The workspace's target/ directory holds
# the output; the crate name is `search-rust`, Cargo emits
# `search_rust.wasm` (underscore form per Cargo's library naming
# convention). Copy with the dash form the integration test expects.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKSPACE_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

(cd "${WORKSPACE_ROOT}" && cargo build --release --target wasm32-unknown-unknown -p search-rust)

cp "${WORKSPACE_ROOT}/target/wasm32-unknown-unknown/release/search_rust.wasm" "${OUT_DIR}/search-rust.wasm"
echo "Wrote ${OUT_DIR}/search-rust.wasm"
