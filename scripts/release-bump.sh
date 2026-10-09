#!/usr/bin/env bash
# Keep every surface that carries the release version in step.
#
# Usage: release-bump.sh set <version>     rewrite the surfaces to <version> (e.g. 5.0.1 or 5.1.0-rc.1)
#        release-bump.sh check <version>   fail unless every surface already equals <version>
#
# Surfaces: release/npm/package.json (npm form), release/pypi/__init__.py and
# release/pypi/ai_rulez/__init__.py (PEP 440 form: 5.1.0-rc.1 -> 5.1.0rc1) and the default of
# `var version` in cmd/ai-rulez/main.go (overridden by -ldflags in release builds, but what
# `go install` users see). The git tag is checked by the publish workflow against the same
# <version>; Homebrew is rendered from the tag and the release checksums at publish time.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

if [ $# -ne 2 ]; then
  echo "Usage: $0 set|check <version>" >&2
  exit 2
fi
mode="$1"
version="${2#v}"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$ ]] || {
  echo "error: '$version' is not X.Y.Z or X.Y.Z-rc.N" >&2
  exit 2
}
py_version="$(printf '%s' "$version" | sed 's/-rc\./rc/')"

npm_json=release/npm/package.json
py_files=(release/pypi/__init__.py release/pypi/ai_rulez/__init__.py)
go_main=cmd/ai-rulez/main.go

current_npm() { sed -n 's/^  "version": "\([^"]*\)".*/\1/p' "$npm_json" | head -n1; }
current_py() { sed -n 's/^__version__ = "\([^"]*\)".*/\1/p' "$1" | head -n1; }
current_go() { sed -n 's/^var version = "\([^"]*\)".*/\1/p' "$go_main" | head -n1; }

case "$mode" in
check)
  bad=0
  expect() {
    if [ "$2" != "$3" ]; then
      echo "::error file=$1::version mismatch in $1: expected $3, found '$2'" >&2
      bad=1
    fi
  }
  expect "$npm_json" "$(current_npm)" "$version"
  for f in "${py_files[@]}"; do expect "$f" "$(current_py "$f")" "$py_version"; done
  expect "$go_main" "$(current_go)" "$version"
  [ "$bad" -eq 0 ] || exit 1
  bash scripts/liter-llm-version.sh >/dev/null
  echo "all version surfaces match $version (PyPI form $py_version; liter-llm $(bash scripts/liter-llm-version.sh))"
  ;;
set)
  # Rewrite through temp files; sed -i differs between GNU and BSD.
  tmp="$(mktemp)"
  trap 'rm -f "$tmp"' EXIT
  sed "s/^  \"version\": \".*\"/  \"version\": \"$version\"/" "$npm_json" >"$tmp" && cat "$tmp" >"$npm_json"
  for f in "${py_files[@]}"; do
    sed "s/^__version__ = \".*\"/__version__ = \"$py_version\"/" "$f" >"$tmp" && cat "$tmp" >"$f"
  done
  sed "s/^var version = \".*\"/var version = \"$version\"/" "$go_main" >"$tmp" && cat "$tmp" >"$go_main"
  bash "$0" check "$version"
  ;;
*)
  echo "Usage: $0 set|check <version>" >&2
  exit 2
  ;;
esac
