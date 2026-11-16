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
  2. Reads the aggrega-abi release the kits depend on: the version
     rust/Cargo.toml requires of the crate aggrega-abi (written once, in its
     [workspace.dependencies]) and the version typescript/package.json names
     for @aggrega-ai/aggrega-abi. Both are read whichever KIT is given.
     Refuses when either names no single X.Y.Z version (a leading ^ or = is
     accepted), resolves from the local filesystem or a git repository, or
     disagrees with the other. The kits and aggrega-abi carry version numbers
     of their own: a kit release names the aggrega-abi release it is built
     against, and this script never changes that dependency.
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

# Prints "<own version> <abi requirement> <abi source>", the last two "-" when
# the crate has no normal dependency on aggrega-abi; the source, which may hold
# spaces, comes last so read gives it the rest of the line. cargo metadata reports a
# dependency inherited from [workspace.dependencies] as the package's own.
read -r rust_own rust_req rust_source < <(node -e '
const m = JSON.parse(require("fs").readFileSync(0, "utf8"));
const p = m.packages.find((p) => p.name === "aggrega-pdk-rust");
if (!p) { console.log("- - -"); process.exit(0); }
const d = p.dependencies.find((d) => d.name === "aggrega-abi" && d.kind === null);
const source = d ? (d.path ? "the local path " + d.path : (d.source || "-")) : "-";
console.log(p.version, d ? d.req : "-", source.replace(/ /g, " "));
' <<<"$metadata")
rust_source="${rust_source//$' '/ }"

# Prints "<own version> <abi spec>", the last "-" when the package has no runtime
# dependency on aggrega-abi.
read -r npm_own npm_spec < <(node -e '
const pkg = JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"));
const spec = (pkg.dependencies || {})["@aggrega-ai/aggrega-abi"];
console.log(pkg.version || "-", spec === undefined ? "-" : spec);
' "$NPM_MANIFEST")

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

# 2. The aggrega-abi release both kits name.

# exact_version SPEC -> the X.Y.Z SPEC names, a leading ^ or = dropped; nothing
# when SPEC names no single X.Y.Z version.
exact_version() {
  local v="$1"
  v="${v#^}"
  v="${v#=}"
  if [[ "$v" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then printf '%s' "$v"; fi
}

refused=0
if [ "$rust_req" = "-" ]; then
  echo "error: $CARGO_MANIFEST declares no dependency on aggrega-abi" >&2
  refused=1
elif [ "$rust_source" != "registry+https://github.com/rust-lang/crates.io-index" ]; then
  echo "error: $CARGO_MANIFEST takes aggrega-abi from $rust_source, not from crates.io" >&2
  refused=1
fi
rust_abi="$(exact_version "$rust_req")"
if [ "$rust_req" != "-" ] && [ -z "$rust_abi" ]; then
  echo "error: $CARGO_MANIFEST requires aggrega-abi '$rust_req', which names no single X.Y.Z release" >&2
  refused=1
fi
npm_abi="$(exact_version "$npm_spec")"
if [ "$npm_spec" = "-" ]; then
  echo "error: $NPM_MANIFEST declares no dependency on @aggrega-ai/aggrega-abi" >&2
  refused=1
elif [ -z "$npm_abi" ]; then
  echo "error: $NPM_MANIFEST names @aggrega-ai/aggrega-abi as \"$npm_spec\", which names no single X.Y.Z release on the npm registry" >&2
  refused=1
fi
if [ -n "$rust_abi" ] && [ -n "$npm_abi" ] && [ "$rust_abi" != "$npm_abi" ]; then
  echo "error: the kits name different aggrega-abi releases: $CARGO_MANIFEST requires $rust_abi, $NPM_MANIFEST names $npm_abi; make them name the same release" >&2
  refused=1
fi
[ "$refused" -eq 0 ] || exit 1
echo "both kits name aggrega-abi $rust_abi" >&2

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

echo "$rust_abi"
