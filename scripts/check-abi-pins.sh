#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Cognitio s.r.l.
#
# check-abi-pins.sh — refuse a tree whose Go, Rust and TypeScript kits pin
# different aggrega-abi releases, and print the one release they all pin. Run
# with --help for the contract.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/check-abi-pins.sh [DIR]

Reads every manifest in the aggrega-pdk tree at DIR (default: the repository
this script belongs to) that pins aggrega-abi, and refuses unless they all pin
the same single release X.Y.Z, taken from its registry:
  Go          every go.mod under go/: the version it requires of
              github.com/aggrega-ai/aggrega-abi/go, its leading v dropped.
              go/go.mod must require it; a replace of that module is refused.
  Rust        every Cargo.toml under rust/, through cargo metadata: the
              requirement each package places on the crate aggrega-abi, a
              requirement inherited from [workspace.dependencies] included,
              so the kit, the workspace's example plugins and the examples
              that are workspaces of their own are all read. The kit's
              package (aggrega-pdk-rust) must depend on it; a requirement
              must be X.Y.Z, ^X.Y.Z or =X.Y.Z and resolve from crates.io.
  TypeScript  every package.json under typescript/: the spec it gives
              @aggrega-ai/aggrega-abi in any dependency list.
              typescript/package.json must name it in its dependencies; a
              spec must be X.Y.Z or =X.Y.Z (a range such as ^X.Y.Z names no
              single release on the npm registry).
A manifest that does not mention aggrega-abi is not read. On refusal every
manifest is named with what it pins.

Reads the manifests and changes nothing. On success it prints the aggrega-abi
version, X.Y.Z, as the one line on standard output; every other message goes
to standard error. Needs cargo and node on PATH.

Exit status:
  0  every manifest pins the same aggrega-abi release; it is printed
  1  refused (the reason and every pin are printed)
  2  usage error, or DIR is not an aggrega-pdk tree
EOF
}

case "${1:-}" in
  -h | --help) usage; exit 0 ;;
esac
if [ "$#" -gt 1 ]; then
  usage >&2
  exit 2
fi
root="${1:-$(dirname "$0")/..}"
if [ ! -f "$root/go/go.mod" ] || [ ! -f "$root/rust/Cargo.toml" ] || [ ! -f "$root/typescript/package.json" ]; then
  echo "error: $root is not an aggrega-pdk tree: go/go.mod, rust/Cargo.toml and typescript/package.json are expected" >&2
  exit 2
fi
cd "$root"

GO_MODULE=github.com/aggrega-ai/aggrega-abi/go
CRATE=aggrega-abi
NPM_PACKAGE=@aggrega-ai/aggrega-abi
CRATES_IO=registry+https://github.com/rust-lang/crates.io-index

# manifests DIR NAME — every file NAME under DIR, outside build output and
# installed dependencies, in a stable order, into the array FILES. A directory
# find cannot read is refused, as a manifest in it would go unread.
manifests() {
  local out
  FILES=()
  if ! out="$(find "$1" \( -name target -o -name node_modules \) -prune -o -type f -name "$2" -print 2>&1)"; then
    REFUSALS+=("could not list every $2 under $1: $(grep -m1 '^find:' <<<"$out" || head -n1 <<<"$out")")
    return
  fi
  [ -z "$out" ] || mapfile -t FILES < <(LC_ALL=C sort <<<"$out")
}

# first_error TEXT — the first line of TEXT naming an error, else its first line.
first_error() { grep -m1 -i 'error' <<<"$1" || head -n1 <<<"$1"; }

# Each pin is one line in PINS: "<manifest><TAB><what it says><TAB><X.Y.Z or empty>",
# the version empty when the pin names no single release. REFUSALS holds every
# other reason to refuse.
PINS=()
REFUSALS=()
# The version each kit's own manifest pins, for the refusal's summary.
declare -A KIT_PIN=()
pin() { PINS+=("$1"$'\t'"$2"$'\t'"$3"); }

# A final release version, X.Y.Z; prefixes are each ecosystem's way of saying
# "this release" and are dropped by the caller.
is_release() { [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; }

# --- Go ---------------------------------------------------------------------------
# go_pins FILE — "require VERSION" for each requirement of GO_MODULE in FILE and
# "replace" for each replace of it, block or single-line form.
go_pins() {
  awk -v m="$GO_MODULE" '
    { sub(/\/\/.*/, "") }
    ($1 == "require" || $1 == "replace") && $2 == "(" { block = $1; next }
    block != "" && $1 == ")" { block = ""; next }
    block == "require" && $1 == m { print "require " $2; next }
    block == "replace" && $1 == m { print "replace"; next }
    $1 == "require" && $2 == m { print "require " $3; next }
    $1 == "replace" && $2 == m { print "replace"; next }
  ' "$1"
}

manifests go go.mod
for f in "${FILES[@]}"; do
  seen=0
  if ! go_out="$(go_pins "$f" 2>&1)"; then
    REFUSALS+=("could not read $f: $(first_error "$go_out")")
    continue
  fi
  while read -r kind version; do
    case "$kind" in
      require)
        seen=1
        v="${version#v}"
        is_release "$v" || v=""
        pin "$f" "requires $GO_MODULE $version" "$v"
        [ "$f" != go/go.mod ] || KIT_PIN[$f]="${v:-$version}"
        ;;
      replace)
        REFUSALS+=("$f replaces $GO_MODULE, so it does not build against a released aggrega-abi; map a local checkout in a go.work instead")
        ;;
    esac
  done <<<"$go_out"
  if [ "$f" = go/go.mod ] && [ "$seen" = 0 ]; then
    REFUSALS+=("go/go.mod requires no version of $GO_MODULE")
  fi
done

# --- Rust -------------------------------------------------------------------------
# cargo metadata reports a whole workspace from any of its manifests, so each
# package is taken once, by its own manifest. Each line printed:
# "<manifest><TAB><package><TAB><kind><TAB><requirement><TAB><source>".
rust_lines=()
declare -A RUST_SEEN=()
kit_has_dep=0
manifests rust Cargo.toml
for f in "${FILES[@]}"; do
  [ -z "${RUST_SEEN[$f]:-}" ] || continue
  # --no-deps reads the manifests alone and resolves nothing, so --offline holds
  # on a machine with no registry cache.
  if ! metadata="$(cargo metadata --no-deps --offline --format-version 1 --manifest-path "$f" 2>&1)"; then
    REFUSALS+=("cargo metadata could not read $f: $(first_error "$metadata")")
    RUST_SEEN[$f]=1
    continue
  fi
  if ! rust_out="$(node -e '
const path = require("path");
const [root, crate] = process.argv.slice(1);
const m = JSON.parse(require("fs").readFileSync(0, "utf8"));
for (const p of m.packages) {
  const rel = path.relative(root, p.manifest_path);
  console.log([rel, "-", "-", "-", "-"].join("\t"));
  for (const d of p.dependencies.filter((d) => d.name === crate)) {
    const source = d.path ? "the local path " + d.path : (d.source || "an unknown source");
    console.log([rel, p.name, d.kind || "normal", d.req, source].join("\t"));
  }
}
' "$(pwd -P)" "$CRATE" <<<"$metadata" 2>&1)"; then
    REFUSALS+=("could not read the cargo metadata of $f: $(first_error "$rust_out")")
    RUST_SEEN[$f]=1
    continue
  fi
  while IFS=$'\t' read -r manifest package kind req source; do
    if [ "$package" = "-" ]; then RUST_SEEN[$manifest]=1; continue; fi
    rust_lines+=("$manifest"$'\t'"$package"$'\t'"$kind"$'\t'"$req"$'\t'"$source")
  done <<<"$rust_out"
done

for line in "${rust_lines[@]}"; do
  IFS=$'\t' read -r manifest package kind req source <<<"$line"
  v="${req#^}"; v="${v#=}"
  is_release "$v" || v=""
  if [ "$package" = aggrega-pdk-rust ] && [ "$kind" = normal ]; then
    kit_has_dep=1
    KIT_PIN[$manifest]="${v:-$req}"
  fi
  what="package $package: $kind dependency $CRATE \"$req\""
  if [ "$source" != "$CRATES_IO" ]; then
    REFUSALS+=("$manifest $what resolves from $source, not from crates.io")
  fi
  pin "$manifest" "$what" "$v"
done
if [ "$kit_has_dep" = 0 ]; then
  REFUSALS+=("rust/Cargo.toml: the package aggrega-pdk-rust declares no dependency on $CRATE")
fi

# --- TypeScript -------------------------------------------------------------------
kit_npm_dep=0
manifests typescript package.json
for f in "${FILES[@]}"; do
  if ! npm_out="$(node -e '
const [file, name] = process.argv.slice(1);
const pkg = JSON.parse(require("fs").readFileSync(file, "utf8"));
for (const list of ["dependencies", "devDependencies", "peerDependencies", "optionalDependencies"]) {
  const spec = (pkg[list] || {})[name];
  if (spec !== undefined) console.log(list + "\t" + spec);
}
' "$f" "$NPM_PACKAGE" 2>&1)"; then
    REFUSALS+=("could not read $f: $(first_error "$npm_out")")
    continue
  fi
  [ -n "$npm_out" ] || continue
  while IFS=$'\t' read -r list spec; do
    v="${spec#=}"
    is_release "$v" || v=""
    if [ "$f" = typescript/package.json ] && [ "$list" = dependencies ]; then
      kit_npm_dep=1
      KIT_PIN[$f]="${v:-$spec}"
    fi
    pin "$f" "$list names $NPM_PACKAGE \"$spec\"" "$v"
  done <<<"$npm_out"
done
if [ "$kit_npm_dep" = 0 ]; then
  REFUSALS+=("typescript/package.json names no $NPM_PACKAGE in its dependencies")
fi

# --- Verdict ----------------------------------------------------------------------
declare -A VERSIONS=()
for p in "${PINS[@]}"; do
  IFS=$'\t' read -r manifest what v <<<"$p"
  if [ -z "$v" ]; then
    REFUSALS+=("$manifest $what, which names no single X.Y.Z release")
  else
    VERSIONS[$v]=1
  fi
done
if [ "${#VERSIONS[@]}" -gt 1 ]; then
  kits=""
  for k in go/go.mod rust/Cargo.toml typescript/package.json; do
    kits+="${kits:+, }$k ${KIT_PIN[$k]:-none}"
  done
  REFUSALS+=("the manifests pin different aggrega-abi releases ($(printf '%s\n' "${!VERSIONS[@]}" | sort -V | paste -sd' ' | sed 's/ /, /g')); the kits pin: $kits; make every manifest below pin the same release")
fi

if [ "${#REFUSALS[@]}" -gt 0 ]; then
  for r in "${REFUSALS[@]}"; do echo "error: $r" >&2; done
  echo "aggrega-abi as each manifest pins it:" >&2
  for p in "${PINS[@]}"; do
    IFS=$'\t' read -r manifest what v <<<"$p"
    printf '  %s  %s  -> %s\n' "$manifest" "$what" "${v:-no single release}" >&2
  done
  exit 1
fi

version="${!VERSIONS[*]}"
echo "the Go, Rust and TypeScript kits pin aggrega-abi $version in all ${#PINS[@]} places that name it" >&2
echo "$version"
