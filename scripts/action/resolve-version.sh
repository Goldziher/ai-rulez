#!/usr/bin/env bash

# Resolve the action's version input to a release tag and write tag=<vX.Y.Z> to $GITHUB_OUTPUT.
# Shared by the root action and its check and update sub-actions, so all three install the same way.
#
# INPUT_VERSION is X.Y.Z, vX.Y.Z, X.Y.Z-rc.N or latest (empty means latest). latest follows
# GitHub's /releases/latest redirect, which names the newest published stable release: never a
# draft or a release candidate.
set -euo pipefail

: "${GITHUB_OUTPUT:?GITHUB_OUTPUT must name the runner output file}"

tag="${INPUT_VERSION:-}"
if [ -z "$tag" ] || [ "$tag" = "latest" ]; then
  resolved="$(curl -fsSL -o /dev/null -w '%{url_effective}' \
    https://github.com/Goldziher/ai-rulez/releases/latest 2>/dev/null || true)"
  tag="${resolved##*/}"
  case "$tag" in
  v*) ;;
  *)
    echo "::error::could not resolve the latest ai-rulez release" >&2
    exit 1
    ;;
  esac
else
  case "$tag" in v*) ;; *) tag="v$tag" ;; esac
fi
if ! [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$ ]]; then
  echo "::error::version must be X.Y.Z, vX.Y.Z, a release candidate, or latest" >&2
  exit 1
fi
echo "tag=$tag" >>"$GITHUB_OUTPUT"
