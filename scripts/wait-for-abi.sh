#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Cognitio s.r.l.
#
# wait-for-abi.sh — wait until a released aggrega-abi version can be installed
# from a registry. Run with --help for the contract.

set -euo pipefail

# Bounded wait: ATTEMPTS checks, INTERVAL seconds apart.
ATTEMPTS=60
INTERVAL=30

usage() {
  cat <<EOF
Usage: scripts/wait-for-abi.sh REGISTRY VERSION

Waits until aggrega-abi VERSION is visible on REGISTRY, so a kit that depends on
it can be published right after aggrega-abi's own release:
  crates.io  the crate aggrega-abi, read from the crates.io sparse index
             (https://index.crates.io/ag/gr/aggrega-abi), which is what cargo
             resolves from
  npm        the package @aggrega-ai/aggrega-abi, read with npm view

VERSION is a final release version, X.Y.Z. The script checks every ${INTERVAL}s
and gives up after $ATTEMPTS checks. Needs curl (crates.io) or npm (npm) on PATH.

Exit status:
  0  VERSION is visible
  1  VERSION did not become visible in time
  2  usage error, or VERSION is not X.Y.Z
EOF
}

case "${1:-}" in
  -h | --help) usage; exit 0 ;;
esac
if [ "$#" -ne 2 ]; then
  usage >&2
  exit 2
fi
registry="$1"
version="$2"
if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: '$version' is not a final release version X.Y.Z" >&2
  exit 2
fi

visible() {
  case "$registry" in
    crates.io)
      # The index file holds one JSON line per published version.
      local index
      index="$(curl -fsS https://index.crates.io/ag/gr/aggrega-abi 2>/dev/null)" || return 1
      grep -qF "\"vers\":\"$version\"" <<<"$index"
      ;;
    npm)
      [ "$(npm view "@aggrega-ai/aggrega-abi@$version" version 2>/dev/null)" = "$version" ]
      ;;
  esac
}

case "$registry" in
  crates.io | npm) ;;
  *)
    echo "error: REGISTRY is crates.io or npm, not '$registry'" >&2
    exit 2
    ;;
esac

for ((attempt = 1; attempt <= ATTEMPTS; attempt++)); do
  if visible; then
    echo "aggrega-abi $version is visible on $registry"
    exit 0
  fi
  if [ "$attempt" -lt "$ATTEMPTS" ]; then
    echo "aggrega-abi $version is not yet visible on $registry (check $attempt of $ATTEMPTS); retrying in ${INTERVAL}s"
    sleep "$INTERVAL"
  fi
done
echo "error: aggrega-abi $version did not become visible on $registry after $ATTEMPTS checks over $((ATTEMPTS * INTERVAL))s; publish aggrega-abi $version there first (its release tag runs aggrega-abi's publish workflow), then re-run this job" >&2
exit 1
