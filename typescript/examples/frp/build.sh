#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build the feature-recipe-pipeline step fixture plugin to a .wasm artifact
# via the Extism JS PDK (extism-js compiler + QuickJS runtime).
#
# Resolution order for the destination directory:
#   1. $AGGREGA_PDK_WASM_OUT  (explicit override; the umbrella's
#      integration test driver may set this when staging into a
#      temporary directory).
#   2. The umbrella's testdata directory, when this PDK is checked
#      out as a submodule of the aggrega-umbrella superproject.
#   3. The PDK's own root testdata/, for genuine standalone checkouts.

set -eo pipefail

# Bring node + extism-js + binaryen onto PATH for non-interactive
# subprocess invocations. The setup-workspace.sh script installs node
# under ~/n/bin and the user-local toolchain binaries under
# ~/.local/bin; both are added here defensively.
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
        echo "  node + npm: ensure ~/n/bin is on PATH per scripts/setup-workspace.sh"
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

# Install fixture-local dependencies (esbuild + types + the PDK the
# guest imports its FRP-step codec from). --no-save keeps
# package-lock.json out of the fixture directory; the PDK's package.json
# has the canonical pin set.
(cd "${SCRIPT_DIR}" && npm install --no-save --silent --no-audit --no-fund)

# Bundle TypeScript to a single CommonJS JS file. --main-fields is set
# explicitly because the neutral platform resolves no package "main"
# field by default, and the guest imports its FRP-step codec from the PDK
# package (whose entry is declared under "main").
(cd "${SCRIPT_DIR}" && npx esbuild src/index.ts --bundle --format=cjs --target=es2022 --platform=neutral --main-fields=module,main --outfile=dist/plugin.js)

# Compile the bundled JS into a Wasm artifact embedding QuickJS. The artifact is
# named for this example's directory and language, the name the umbrella's
# fixture builder resolves back to this directory.
extism-js -i "${SCRIPT_DIR}/index.d.ts" "${SCRIPT_DIR}/dist/plugin.js" -o "${OUT_DIR}/frp-typescript.wasm"
echo "Wrote ${OUT_DIR}/frp-typescript.wasm"
