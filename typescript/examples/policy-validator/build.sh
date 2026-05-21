#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the policy-validator fixture plugin to a .wasm artifact via
# the Extism JS PDK (extism-js compiler + QuickJS runtime).

set -eo pipefail

if [ -d "${HOME}/n/bin" ]; then
    export PATH="${HOME}/n/bin:${PATH}"
fi
if [ -d "${HOME}/.local/bin" ]; then
    export PATH="${HOME}/.local/bin:${PATH}"
fi

for tool in node npm extism-js wasm-merge wasm-opt; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "error: required tool '$tool' not on PATH for the TypeScript fixture build."
        echo "  See the mutator fixture's build.sh for install pointers."
        exit 1
    fi
done

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

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "${OUT_DIR}" "${SCRIPT_DIR}/dist"

(cd "${SCRIPT_DIR}" && npm install --no-save --silent --no-audit --no-fund)
(cd "${SCRIPT_DIR}" && npx esbuild src/index.ts --bundle --format=cjs --target=es2022 --platform=neutral --outfile=dist/plugin.js)
extism-js -i "${SCRIPT_DIR}/index.d.ts" "${SCRIPT_DIR}/dist/plugin.js" -o "${OUT_DIR}/policy-validator-typescript.wasm"
echo "Wrote ${OUT_DIR}/policy-validator-typescript.wasm"
