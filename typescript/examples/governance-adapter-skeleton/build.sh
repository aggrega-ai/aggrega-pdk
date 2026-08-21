#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build this adapter skeleton to a .wasm artifact via the Extism JS PDK
# (the extism-js compiler and the QuickJS runtime it embeds).
#
# The artifact lands in dist/ beside this script, which is where a plugin author
# building their own adapter wants it. $AGGREGA_PDK_WASM_OUT overrides the
# destination when a caller stages the artifact somewhere else.

set -eo pipefail

if [ -d "${HOME}/n/bin" ]; then
    export PATH="${HOME}/n/bin:${PATH}"
fi
if [ -d "${HOME}/.local/bin" ]; then
    export PATH="${HOME}/.local/bin:${PATH}"
fi

for tool in node npm extism-js wasm-merge wasm-opt; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "error: required tool '$tool' is not on PATH."
        echo "  extism-js: github.com/extism/js-pdk/releases"
        echo "  wasm-merge, wasm-opt: github.com/WebAssembly/binaryen/releases"
        exit 1
    fi
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUT_DIR="${AGGREGA_PDK_WASM_OUT:-${SCRIPT_DIR}/dist}"
ARTIFACT="governance-adapter-skeleton-typescript.wasm"

mkdir -p "${OUT_DIR}" "${SCRIPT_DIR}/dist"

(cd "${SCRIPT_DIR}" && npm install --no-save --silent --no-audit --no-fund)
(cd "${SCRIPT_DIR}" && npx esbuild src/index.ts --bundle --format=cjs --target=es2022 \
    --platform=neutral --main-fields=module,main --outfile=dist/plugin.js)
extism-js -i "${SCRIPT_DIR}/index.d.ts" "${SCRIPT_DIR}/dist/plugin.js" -o "${OUT_DIR}/${ARTIFACT}"
echo "Wrote ${OUT_DIR}/${ARTIFACT}"
