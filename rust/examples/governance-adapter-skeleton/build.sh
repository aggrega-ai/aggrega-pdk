#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build this adapter skeleton to a .wasm artifact.
#
# The artifact lands in dist/ beside this script, which is where a plugin author
# building their own adapter wants it. $AGGREGA_PDK_WASM_OUT overrides the
# destination when a caller stages the artifact somewhere else.

set -eo pipefail

if [ -f "${HOME}/.cargo/env" ]; then
    # shellcheck disable=SC1091
    . "${HOME}/.cargo/env"
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKSPACE_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
OUT_DIR="${AGGREGA_PDK_WASM_OUT:-${SCRIPT_DIR}/dist}"
CRATE="governance-adapter-skeleton-rust"
ARTIFACT="governance-adapter-skeleton-rust.wasm"

mkdir -p "${OUT_DIR}"

# Built from the enclosing plugin workspace, which this crate is a member of: it
# has no engine dependency of its own, so it resolves through the shared lockfile
# rather than needing an isolated tree. An adapter whose engine needs a newer
# toolchain becomes its own workspace instead.
(cd "${WORKSPACE_ROOT}" && cargo build --release --target wasm32-wasip1 -p "${CRATE}")

cp "${WORKSPACE_ROOT}/target/wasm32-wasip1/release/governance_adapter_skeleton_rust.wasm" \
    "${OUT_DIR}/${ARTIFACT}"
echo "Wrote ${OUT_DIR}/${ARTIFACT}"
