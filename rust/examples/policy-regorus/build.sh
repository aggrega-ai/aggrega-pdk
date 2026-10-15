#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the policy-regorus example plugin to a .wasm artifact.
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

BUILT="${CRATE_DIR}/target/wasm32-wasip1/release/policy_regorus_rust.wasm"
DEST="${OUT_DIR}/policy-regorus-rust.wasm"

# Post-build size optimisation. wasm-opt shrinks the artifact further than the
# compiler alone; the feature flags enumerate exactly the WebAssembly proposals
# the wasm32-wasip1 output relies on, so the optimiser validates rather than
# rejects the module. The pass is deterministic for a given input and optimiser
# version. When wasm-opt is unavailable (standalone checkouts without the
# binaryen toolchain), fall back to copying the compiler output unchanged.
if command -v wasm-opt >/dev/null 2>&1; then
    wasm-opt -Oz \
        --enable-bulk-memory \
        --enable-sign-ext \
        --enable-simd \
        --enable-nontrapping-float-to-int \
        "${BUILT}" -o "${DEST}"
else
    echo "wasm-opt not found on PATH; copying unoptimised artifact" >&2
    cp "${BUILT}" "${DEST}"
fi
echo "Wrote ${DEST}"
