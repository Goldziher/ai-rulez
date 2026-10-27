#!/usr/bin/env bash

# Install the ai-rulez release named by AI_RULEZ_VERSION (a tag from resolve-version.sh) into
# AI_RULEZ_CACHE_DIR, verified against the release's checksums.txt, then add its directory to
# $GITHUB_PATH and write version= and path= to $GITHUB_OUTPUT. Shared by the root action and its
# check and update sub-actions; the download and verification are scripts/pre-commit/run-ai-rulez.sh.
set -euo pipefail

: "${GITHUB_OUTPUT:?GITHUB_OUTPUT must name the runner output file}"
: "${GITHUB_PATH:?GITHUB_PATH must name the runner path file}"
: "${AI_RULEZ_VERSION:?AI_RULEZ_VERSION must name the release tag to install}"
: "${AI_RULEZ_CACHE_DIR:?AI_RULEZ_CACHE_DIR must name the download cache directory}"

cache_dir="$AI_RULEZ_CACHE_DIR"
if command -v cygpath >/dev/null 2>&1; then
  cache_dir="$(cygpath -u "$AI_RULEZ_CACHE_DIR")"
fi
export AI_RULEZ_CACHE_DIR="$cache_dir"
script="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../pre-commit" && pwd)/run-ai-rulez.sh"
binary_path="$(bash "$script")"
dir="$(dirname "$binary_path")"
native_dir="$dir"
if command -v cygpath >/dev/null 2>&1; then native_dir="$(cygpath -w "$dir")"; fi
echo "$native_dir" >>"$GITHUB_PATH"
echo "version=${AI_RULEZ_VERSION#v}" >>"$GITHUB_OUTPUT"
echo "path=$binary_path" >>"$GITHUB_OUTPUT"
echo "ai-rulez ${AI_RULEZ_VERSION} installed at $binary_path" >&2
