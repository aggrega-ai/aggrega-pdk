#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the mutator fixture plugin to a .wasm artifact via the
# Extism JS PDK (extism-js compiler + QuickJS runtime).
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override; the aggrega-cli test
#      harness sets this when it builds this example as a fixture,
#      staging into a temporary directory).
#   2. Otherwise, the PDK's own root testdata/.

set -eo pipefail

# Bring node + extism-js + binaryen onto PATH for non-interactive
# subprocess invocations. Node under ~/n/bin and the user-local
# toolchain binaries under ~/.local/bin are both added here
# defensively, each only when its directory exists.
if [ -d "${HOME}/n/bin" ]; then
    export PATH="${HOME}/n/bin:${PATH}"
fi
if [ -d "${HOME}/.local/bin" ]; then
    export PATH="${HOME}/.local/bin:${PATH}"
fi

# Verify the required tools are present. Fail fast with a clear
# diagnostic if any is missing so the failure points at the right
# install action.
for tool in node npm extism-js wasm-merge wasm-opt; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "error: required tool '$tool' not on PATH for the TypeScript fixture build."
        echo "  extism-js: install from github.com/extism/js-pdk/releases (v1.6.0 line)"
        echo "    curl -fsSL -o /tmp/extism-js.gz https://github.com/extism/js-pdk/releases/download/v1.6.0/extism-js-x86_64-linux-v1.6.0.gz"
        echo "    gunzip -f /tmp/extism-js.gz && chmod +x /tmp/extism-js"
        echo "    mv /tmp/extism-js ~/.local/bin/extism-js"
        echo "  wasm-merge + wasm-opt: install via the binaryen release tarball"
        echo "    curl -fsSL -o /tmp/binaryen.tar.gz https://github.com/WebAssembly/binaryen/releases/download/version_129/binaryen-version_129-x86_64-linux.tar.gz"
        echo "    mkdir -p ~/.local/share/binaryen-version_129 && tar -C ~/.local/share/binaryen-version_129 --strip-components=1 -xzf /tmp/binaryen.tar.gz"
        echo "    ln -sf ~/.local/share/binaryen-version_129/bin/wasm-merge ~/.local/bin/wasm-merge"
        echo "    ln -sf ~/.local/share/binaryen-version_129/bin/wasm-opt ~/.local/bin/wasm-opt"
        echo "  node + npm: ensure both are on PATH (this script adds ~/n/bin when it exists)"
        exit 1
    fi
done

if [ -n "${AGGREGA_PDK_WASM_OUT:-}" ]; then
    OUT_DIR="${AGGREGA_PDK_WASM_OUT}"
else
    OUT_DIR="$(git rev-parse --show-toplevel)/testdata"
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "${OUT_DIR}" "${SCRIPT_DIR}/dist"

# Install fixture-local dependencies (esbuild + types). --no-save
# keeps package-lock.json out of the fixture directory; the PDK's
# package.json has the canonical pin set.
(cd "${SCRIPT_DIR}" && npm install --no-save --silent --no-audit --no-fund)

# Bundle TypeScript to a single CommonJS JS file.
(cd "${SCRIPT_DIR}" && npx esbuild src/index.ts --bundle --format=cjs --target=es2022 --platform=neutral --main-fields=module,main --outfile=dist/plugin.js)

# Compile the bundled JS into a Wasm artifact embedding QuickJS.
extism-js -i "${SCRIPT_DIR}/index.d.ts" "${SCRIPT_DIR}/dist/plugin.js" -o "${OUT_DIR}/mutator-typescript.wasm"
echo "Wrote ${OUT_DIR}/mutator-typescript.wasm"
