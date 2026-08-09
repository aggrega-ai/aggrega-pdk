#!/usr/bin/env bash
# Type-check every example plugin under examples/, each on its OWN tsconfig.json.
#
# The examples are the surface a plugin author copies from first, so a broken one
# costs more than a broken internal file: the author assumes it compiles and
# debugs their own code instead. What this step checks is therefore the
# configuration each example SHIPS WITH, rather than a second aggregating
# configuration that resembles it — a check run under options the example does
# not carry can pass over sources that fail the moment somebody copies them out.
#
# The example set is DISCOVERED by glob rather than listed, so an example added
# tomorrow is checked without editing this script, and a check that discovers
# nothing refuses rather than reporting success over an empty corpus.
#
# Dependency resolution. Each example resolves
# @aggrega-ai/aggrega-pdk-typescript through its own node_modules, where npm
# links it to this package. @extism/js-pdk and the compiler itself resolve
# UPWARD from this package's node_modules, so that link is the only thing an
# example needs installed — measured by moving one example's node_modules aside,
# which left exactly one error, the unresolved link.
#
# The link is installed with the same invocation the example's own build.sh
# uses, and only when the directory is absent. Installing unconditionally would
# add several times this package's whole build time to every mode that reaches
# it, and it would buy nothing: the link points at the live tree so it cannot go
# stale, @extism/js-pdk is pinned to an exact version, and the compiler used here
# is this package's own rather than the example's copy.

set -euo pipefail

cd "$(dirname "$0")/.."

shopt -s nullglob
configs=(examples/*/tsconfig.json)
if [ "${#configs[@]}" -eq 0 ]; then
  echo "error: no example tsconfig.json found under examples/ — this step would certify nothing" >&2
  exit 1
fi

for config in "${configs[@]}"; do
  dir="$(dirname "$config")"
  if [ ! -d "$dir/node_modules" ]; then
    ( cd "$dir" && npm install --no-save --silent --no-audit --no-fund )
  fi
  tsc -p "$config"
done

echo "type-checked ${#configs[@]} example plugins on their own configurations"
