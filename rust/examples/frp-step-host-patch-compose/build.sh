#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the reference cross-entity (/host PATCH) feature-recipe-pipeline
# step plugin for the compose provider to a .wasm artifact. Where the
# helm-chart variant patches chart source per-node, this plugin patches the
# compose provider's MERGED docker-compose.yaml in the post-assembly phase.
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override).
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

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKSPACE_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

(cd "${WORKSPACE_ROOT}" && cargo build --release --target wasm32-unknown-unknown -p frp-step-host-patch-compose-rust)

cp "${WORKSPACE_ROOT}/target/wasm32-unknown-unknown/release/frp_step_host_patch_compose_rust.wasm" "${OUT_DIR}/frp-step-host-patch-compose-rust.wasm"
echo "Wrote ${OUT_DIR}/frp-step-host-patch-compose-rust.wasm"
