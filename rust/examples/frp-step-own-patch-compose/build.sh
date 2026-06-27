#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the reference own-scope (/own PATCH) feature-recipe-pipeline step
# plugin for the compose provider to a .wasm artifact. The compose provider is
# an assembler: a component's own per-node output is a pre-assembly fragment,
# merged into one docker-compose.yaml only at assembly. A Recipe's /own pipeline
# therefore cannot run per-node; the host defers it and replays this plugin in
# the post-assembly phase against the MERGED docker-compose.yaml.
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override).
#   2. The umbrella's renderer pipeline testdata directory, when this PDK
#      is checked out as a submodule of the aggrega-umbrella superproject.
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

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKSPACE_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

(cd "${WORKSPACE_ROOT}" && cargo build --release --target wasm32-unknown-unknown -p frp-step-own-patch-compose-rust)

cp "${WORKSPACE_ROOT}/target/wasm32-unknown-unknown/release/frp_step_own_patch_compose_rust.wasm" "${OUT_DIR}/frp-step-own-patch-compose-rust.wasm"
echo "Wrote ${OUT_DIR}/frp-step-own-patch-compose-rust.wasm"
