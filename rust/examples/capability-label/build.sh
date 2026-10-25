#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the capability-label plugin to a .wasm artifact.
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override, for a build staged into a
#      temporary directory).
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

# Build the cdylib via Cargo. The workspace's target/ directory holds the
# output; the crate name is `capability-label-rust`, which Cargo emits as
# `capability_label_rust.wasm` (underscore form per Cargo's library naming
# convention). Copy it out with the dash form.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKSPACE_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

(cd "${WORKSPACE_ROOT}" && cargo build --release --target wasm32-unknown-unknown -p capability-label-rust)

TARGET_DIR="${CARGO_TARGET_DIR:-${WORKSPACE_ROOT}/target}"
cp "${TARGET_DIR}/wasm32-unknown-unknown/release/capability_label_rust.wasm" "${OUT_DIR}/capability-label-rust.wasm"
echo "Wrote ${OUT_DIR}/capability-label-rust.wasm"
