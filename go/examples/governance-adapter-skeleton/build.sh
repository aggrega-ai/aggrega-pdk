#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Build this adapter skeleton to a .wasm artifact.
#
# The artifact lands in dist/ beside this script, which is where a plugin author
# building their own adapter wants it. $AGGREGA_PDK_WASM_OUT overrides the
# destination when a caller stages the artifact somewhere else.

set -eo pipefail

# This example is a standalone module (its own go.mod + replace directives)
# rather than a member of an enclosing Go workspace. Disabling the workspace
# makes the build resolve dependencies through this module, so it behaves the
# same whether or not it was copied out of one.
export GOWORK=off

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUT_DIR="${AGGREGA_PDK_WASM_OUT:-${SCRIPT_DIR}/dist}"
ARTIFACT="governance-adapter-skeleton-go.wasm"

# The version the guest self-reports through describe(). The linker folds it into
# the descriptor's metadata, so the running guest announces its own version at
# load, and the publish step cross-checks that self-report against the version
# the plugin entity declares. Commit is deliberately left unstamped: a dynamic
# commit hash would make the artifact differ between builds of identical source.
PLUGIN_VERSION="${AGGREGA_PLUGIN_VERSION:-0.1.0}"

mkdir -p "${OUT_DIR}"
(cd "${SCRIPT_DIR}" && GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared \
    -ldflags "-X github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega.Version=${PLUGIN_VERSION}" \
    -o "${OUT_DIR}/${ARTIFACT}" ./)
echo "Wrote ${OUT_DIR}/${ARTIFACT}"
