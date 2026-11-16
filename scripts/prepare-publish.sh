#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Cognitio s.r.l.
#
# prepare-publish.sh — make a kit's manifest name the released aggrega-abi it is
# published with, and refuse to publish what a registry user could not install.
# Run with --help for the contract.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/prepare-publish.sh KIT VERSION

KIT is rust (rust/Cargo.toml, crate aggrega-pdk-rust) or typescript
(typescript/package.json, npm package @aggrega-ai/aggrega-pdk-typescript).
VERSION is a final release version, X.Y.Z with no pre-release or build suffix
(a release tag vX.Y.Z without its leading v).

In order, for the KIT's manifest:
  1. Refuses when the manifest's own version is not VERSION.
  2. Sets the dependency on aggrega-abi to exactly VERSION, whatever it names
     now: a local path, a file: or link: spec, or another version. The kits
     are released together with aggrega-abi at the same version.
       rust:       cargo add aggrega-abi@VERSION, run only when the requirement
                   is not already VERSION from crates.io; it queries crates.io
                   and updates Cargo.lock.
       typescript: npm pkg set; run npm install afterwards to update the lock.
  3. Refuses when any dependency (of every kind) still resolves from the local
     filesystem or a git repository rather than the registry: a Cargo path or
     git dependency, an npm file:, link:, workspace:, portal: or path spec.
     Each one is named.
  4. rust only: refuses when the git checkout differs from its commit anywhere
     but rust/Cargo.toml and rust/Cargo.lock (untracked files included), so
     cargo package and cargo publish may run with --allow-dirty and still
     publish the release tag plus the dependency rewrite and nothing else.

Run from anywhere; paths resolve against the repository root. Needs cargo and git (rust)
or npm (typescript), and node, on PATH. The manifest is edited in place.

Exit status:
  0  the manifest is ready to publish
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

cd "$(dirname "$0")/.."

# The own-version check has the same shape as the one aggrega-abi runs before it
# publishes; each repository carries its own because each one builds and
# publishes on its own.

prepare_rust() {
  local metadata info own req source
  metadata="$(cargo metadata --no-deps --format-version 1 --manifest-path rust/Cargo.toml)"

  # Prints "<own version> <abi requirement> <abi source>", the last two "-" when
  # there is no normal dependency on aggrega-abi.
  info="$(node -e '
const m = JSON.parse(require("fs").readFileSync(0, "utf8"));
const p = m.packages.find((p) => p.name === "aggrega-pdk-rust");
if (!p) { console.error("error: rust/Cargo.toml declares no aggrega-pdk-rust package"); process.exit(1); }
const d = p.dependencies.find((d) => d.name === "aggrega-abi" && d.kind === null);
console.log(p.version, d ? d.req : "-", d ? (d.path ? "path" : (d.source || "-")) : "-");
' <<<"$metadata")"
  read -r own req source <<<"$info"

  if [ "$own" != "$version" ]; then
    echo "error: rust/Cargo.toml carries version $own, not the release version $version" >&2
    return 1
  fi
  if [ "$req" = "-" ]; then
    echo "error: rust/Cargo.toml declares no dependency on aggrega-abi" >&2
    return 1
  fi
  if [ "$req" = "^$version" ] && [ "$source" = "registry+https://github.com/rust-lang/crates.io-index" ]; then
    echo "rust/Cargo.toml already requires aggrega-abi $version from crates.io"
  else
    echo "rust/Cargo.toml requires aggrega-abi '$req' from '$source'; setting it to $version from crates.io"
    cargo add --manifest-path rust/Cargo.toml --package aggrega-pdk-rust "aggrega-abi@$version"
    metadata="$(cargo metadata --no-deps --format-version 1 --manifest-path rust/Cargo.toml)"
  fi

  node -e '
const m = JSON.parse(require("fs").readFileSync(0, "utf8"));
const p = m.packages.find((p) => p.name === "aggrega-pdk-rust");
const local = p.dependencies.filter((d) => d.path || (d.source && d.source.startsWith("git+")));
for (const d of local) {
  console.error("error: rust/Cargo.toml " + (d.kind || "normal") + " dependency " + d.name +
    " resolves from " + (d.path ? "the local path " + d.path : d.source) + ", not from crates.io");
}
if (local.length) process.exit(1);
console.log("rust/Cargo.toml names every dependency from crates.io");
' <<<"$metadata"

  # cargo package and cargo publish refuse a checkout with uncommitted changes
  # unless given --allow-dirty, and the rewrite above is such a change. The
  # publish runs with --allow-dirty only because this check holds: the manifest
  # and its lock file are the only paths that differ from the release tag.
  local dirty unexpected path
  if ! dirty="$(git status --porcelain --untracked-files=all 2>/dev/null)"; then
    echo "error: $(pwd) is not a git checkout; the crate is published from a checkout of its release tag" >&2
    return 1
  fi
  unexpected="$(cut -c4- <<<"$dirty" | grep -vxE 'rust/Cargo\.(toml|lock)' || true)"
  if [ -n "$unexpected" ]; then
    echo "error: the checkout differs from the release tag beyond rust/Cargo.toml and rust/Cargo.lock; refusing to publish it with --allow-dirty:" >&2
    while IFS= read -r path; do echo "  $path" >&2; done <<<"$unexpected"
    return 1
  fi
  echo "the checkout differs from the release tag only in rust/Cargo.toml and rust/Cargo.lock, if at all"
}

prepare_typescript() {
  local manifest=typescript/package.json
  local sections
  # Prints the own version, then each dependency section naming aggrega-abi.
  sections="$(node -e '
const pkg = JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"));
const kinds = ["dependencies", "optionalDependencies", "peerDependencies", "devDependencies"];
console.log(pkg.version);
for (const k of kinds) if (pkg[k] && Object.hasOwn(pkg[k], "@aggrega-ai/aggrega-abi")) console.log(k + " " + pkg[k]["@aggrega-ai/aggrega-abi"]);
' "$manifest")"

  local own
  own="$(head -n 1 <<<"$sections")"
  if [ "$own" != "$version" ]; then
    echo "error: $manifest carries version $own, not the release version $version" >&2
    return 1
  fi
  local found=0 kind spec
  while read -r kind spec; do
    [ -n "$kind" ] || continue
    found=1
    if [ "$spec" = "$version" ]; then
      echo "$manifest $kind already names @aggrega-ai/aggrega-abi $version"
    else
      echo "$manifest $kind names @aggrega-ai/aggrega-abi '$spec'; setting it to $version"
      (cd typescript && npm pkg set "$kind.@aggrega-ai/aggrega-abi=$version")
    fi
  done < <(tail -n +2 <<<"$sections")
  if [ "$found" -eq 0 ]; then
    echo "error: $manifest declares no dependency on @aggrega-ai/aggrega-abi" >&2
    return 1
  fi

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
console.log(process.argv[1] + " names every dependency from the npm registry");
' "$manifest"
}

case "$kit" in
  rust) prepare_rust ;;
  typescript) prepare_typescript ;;
  *)
    echo "error: KIT is rust or typescript, not '$kit'" >&2
    exit 2
    ;;
esac
