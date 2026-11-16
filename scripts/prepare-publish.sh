#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Cognitio s.r.l.
#
# prepare-publish.sh — confirm a kit is ready to publish at a release tag, name
# the aggrega-abi release it depends on, and refuse to publish what a registry
# user could not install. Run with --help for the contract.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/prepare-publish.sh KIT VERSION

KIT is rust (rust/Cargo.toml, crate aggrega-pdk-rust) or typescript
(typescript/package.json, npm package @aggrega-ai/aggrega-pdk-typescript).
VERSION is a final release version, X.Y.Z with no pre-release or build suffix
(a release tag vX.Y.Z without its leading v).

Reads the manifests and changes nothing. In order:
  1. Refuses when the KIT's manifest does not carry VERSION as its own
     version.
  2. Reads the aggrega-abi release the kits depend on with
     scripts/check-abi-pins.sh, whichever KIT is given: it refuses unless the
     Go, Rust and TypeScript manifests all pin the same single X.Y.Z release
     from their registries (see its --help). The kits and aggrega-abi carry
     version numbers of their own: a kit release names the aggrega-abi
     release it is built against, and this script never changes that
     dependency.
  3. Refuses when any dependency of the KIT (of every kind) resolves from the
     local filesystem or a git repository rather than the registry: a Cargo
     path or git dependency, an npm file:, link:, workspace:, portal: or path
     spec. Each one is named.

On success it prints the aggrega-abi version, X.Y.Z, as the one line on
standard output; every other message goes to standard error.

Run from anywhere; paths resolve against the repository root. Needs cargo and
node on PATH.

Exit status:
  0  the KIT is ready to publish; the aggrega-abi version is printed
  1  refused (the reason is printed)
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
kit="$1"
version="$2"
if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: '$version' is not a final release version X.Y.Z" >&2
  exit 2
fi
case "$kit" in
  rust | typescript) ;;
  *)
    echo "error: KIT is rust or typescript, not '$kit'" >&2
    exit 2
    ;;
esac

cd "$(dirname "$0")/.."

CARGO_MANIFEST=rust/Cargo.toml
NPM_MANIFEST=typescript/package.json

# The own-version check has the same shape as the one aggrega-abi runs before it
# publishes; each repository carries its own because each one builds and
# publishes on its own.

metadata="$(cargo metadata --no-deps --format-version 1 --manifest-path "$CARGO_MANIFEST")"

# Prints the version of the package aggrega-pdk-rust, "-" when the manifest
# declares none.
rust_own="$(node -e '
const m = JSON.parse(require("fs").readFileSync(0, "utf8"));
const p = m.packages.find((p) => p.name === "aggrega-pdk-rust");
console.log(p ? p.version : "-");
' <<<"$metadata")"

# Prints the package's own version, "-" when it declares none.
npm_own="$(node -e '
const pkg = JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"));
console.log(pkg.version || "-");
' "$NPM_MANIFEST")"

if [ "$rust_own" = "-" ]; then
  echo "error: $CARGO_MANIFEST declares no aggrega-pdk-rust package" >&2
  exit 1
fi

# 1. The KIT's own version.
case "$kit" in
  rust) own="$rust_own" manifest="$CARGO_MANIFEST" ;;
  typescript) own="$npm_own" manifest="$NPM_MANIFEST" ;;
esac
if [ "$own" != "$version" ]; then
  echo "error: $manifest carries version $own, not the release version $version" >&2
  exit 1
fi
echo "$manifest carries the release version $version" >&2

# 2. The aggrega-abi release the kits pin, the same in every manifest.
abi="$(scripts/check-abi-pins.sh)" || exit 1

# 3. Every dependency of the KIT from its registry.
case "$kit" in
  rust)
    node -e '
const m = JSON.parse(require("fs").readFileSync(0, "utf8"));
const p = m.packages.find((p) => p.name === "aggrega-pdk-rust");
const local = p.dependencies.filter((d) => d.path || (d.source && d.source.startsWith("git+")));
for (const d of local) {
  console.error("error: rust/Cargo.toml " + (d.kind || "normal") + " dependency " + d.name +
    " resolves from " + (d.path ? "the local path " + d.path : d.source) + ", not from crates.io");
}
if (local.length) process.exit(1);
console.error("rust/Cargo.toml names every dependency from crates.io");
' <<<"$metadata"
    ;;
  typescript)
    node -e '
const pkg = JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"));
const kinds = ["dependencies", "optionalDependencies", "peerDependencies", "devDependencies"];
const localSpec = /^(file:|link:|workspace:|portal:|\.{1,2}(\/|$)|\/|~\/)/;
let refused = 0;
for (const k of kinds) for (const [name, spec] of Object.entries(pkg[k] || {})) {
  if (localSpec.test(spec)) {
    console.error("error: " + process.argv[1] + " " + k + " names " + name + " as \"" + spec + "\", which resolves from the local filesystem, not from the npm registry");
    refused = 1;
  }
}
if (refused) process.exit(1);
console.error(process.argv[1] + " names every dependency from the npm registry");
' "$NPM_MANIFEST"
    ;;
esac

echo "$abi"
