#!/usr/bin/env bash
# Print the liter-llm version (without the leading "v") that the Go build pins.
#
# go.mod is the single source of truth: the native static library fetched at release time is the
# asset of the same release as the Go binding. Every go.mod in the repo that requires a
# github.com/xberg-io/liter-llm module (the root module and internal/llm/literllm) must agree; a
# disagreement fails instead of silently picking one.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mods=("$root/go.mod")
[ -f "$root/internal/llm/literllm/go.mod" ] && mods+=("$root/internal/llm/literllm/go.mod")

versions=""
for mod in "${mods[@]}"; do
  # Matches `require <mod> vX` and `<mod> vX` inside a require block.
  found="$(awk '
    $1 == "require" && $2 ~ /^github.com\/xberg-io\/liter-llm(\/|$)/ { print $3 }
    $1 ~ /^github.com\/xberg-io\/liter-llm(\/|$)/ && $2 ~ /^v[0-9]/ { print $2 }
  ' "$mod")"
  versions+="${found}"$'\n'
done

unique="$(printf '%s' "$versions" | sed '/^$/d' | sort -u)"
count="$(printf '%s' "$unique" | grep -c . || true)"
if [ "$count" -eq 0 ]; then
  echo "error: no github.com/xberg-io/liter-llm requirement found in ${mods[*]}" >&2
  exit 1
fi
if [ "$count" -ne 1 ]; then
  echo "error: liter-llm version disagrees across go.mod files: $(printf '%s' "$unique" | tr '\n' ' ')" >&2
  exit 1
fi

version="${unique#v}"
case "$version" in
*-*)
  echo "error: liter-llm version '$unique' is a pre-release/pseudo-version; the native asset only exists for releases" >&2
  exit 1
  ;;
[0-9]*.[0-9]*.[0-9]*) ;;
*)
  echo "error: unsupported liter-llm version '$unique' (need a released vMAJOR.MINOR.PATCH)" >&2
  exit 1
  ;;
esac
printf '%s\n' "$version"
