#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the policy-kubewarden-render example plugin to a .wasm artifact.
#
# This crate is a self-contained workspace (see Cargo.toml) built under its
# own pinned toolchain, so the build runs from this directory rather than the
# enclosing plugin workspace root, and dispatches through `rustup run stable`.
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override; the aggrega-cli test harness
#      sets this when it builds this example as a fixture, staging into a
#      temporary directory).
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

CRATE_DIR="$(cd "$(dirname "$0")" && pwd)"

(cd "${CRATE_DIR}" && rustup run stable cargo build --release --target wasm32-wasip1)

cp "${CRATE_DIR}/target/wasm32-wasip1/release/policy_kubewarden_render_rust.wasm" "${OUT_DIR}/policy-kubewarden-render-rust.wasm"
echo "Wrote ${OUT_DIR}/policy-kubewarden-render-rust.wasm"
