#!/usr/bin/env bash

# sync-xberg-bats.sh refreshes the vendored xberg-bats helper library from xberg-io/actions,
# or (--check) fails when the committed copy has drifted from the pinned commit.
#
#   sync-xberg-bats.sh              re-copy load.bash at the pinned ref
#   sync-xberg-bats.sh --ref v1     re-pin to a tag, branch or commit, then copy
#   sync-xberg-bats.sh --check      exit 2 when the copy differs from the pinned ref
#
# XBERG_ACTIONS_URL overrides the repository (a URL or a local checkout).
# Exit codes: 0 in sync or synced, 1 could not run, 2 drift.

set -euo pipefail

root="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"
pin_file="${root}/tests/lib/xberg-bats.pin"
lib_file="${root}/tests/lib/xberg-bats/load.bash"
upstream_path="bats-lib/xberg-bats/load.bash"

die() {
  echo "sync-xberg-bats: $*" >&2
  exit 1
}

usage() {
  sed -n '3,11p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    die "neither sha256sum nor shasum is available"
  fi
}

pin_value() {
  local key="$1" value
  value="$(sed -n "s/^${key}=//p" "$pin_file" | tail -n 1)"
  [[ -n "$value" ]] || die "${pin_file} has no ${key}= line"
  printf '%s' "$value"
}

check=0
requested=""
while [[ $# -gt 0 ]]; do
  case "$1" in
  --check) check=1 ;;
  --ref)
    [[ $# -ge 2 && -n "$2" ]] || die "--ref needs a value"
    requested="$2"
    shift
    ;;
  -h | --help)
    usage
    exit 0
    ;;
  *) die "unknown argument: $1 (see --help)" ;;
  esac
  shift
done

[[ -f "$pin_file" ]] || die "missing pin file ${pin_file}"
command -v git >/dev/null 2>&1 || die "git is required"

if [[ "$check" -eq 1 && -n "$requested" ]]; then
  die "--check verifies the pinned ref; it does not take --ref"
fi

url="${XBERG_ACTIONS_URL:-$(pin_value repo)}"
ref="$(pin_value ref)"
fetch_ref="${requested:-$ref}"

if [[ "$check" -eq 1 ]]; then
  # The recorded digest catches a hand edit even before the network is reached.
  [[ -f "$lib_file" ]] || {
    echo "sync-xberg-bats: ${lib_file} is missing; run task test:lib:sync" >&2
    exit 2
  }
  if [[ "$(sha256_of "$lib_file")" != "$(pin_value sha256)" ]]; then
    echo "sync-xberg-bats: ${lib_file} was edited by hand (sha256 differs from the pin); run task test:lib:sync" >&2
    exit 2
  fi
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

git -C "$work" init -q
git -C "$work" fetch -q --depth 1 "$url" "$fetch_ref" ||
  die "could not fetch ${fetch_ref} from ${url}"
commit="$(git -C "$work" rev-parse 'FETCH_HEAD^{commit}')"
git -C "$work" show "${commit}:${upstream_path}" >"${work}/load.bash" ||
  die "${upstream_path} does not exist at ${commit}"

if [[ "$check" -eq 1 ]]; then
  if ! cmp -s "${work}/load.bash" "$lib_file"; then
    echo "sync-xberg-bats: ${lib_file} differs from ${upstream_path} at ${ref}; run task test:lib:sync" >&2
    diff -u "$lib_file" "${work}/load.bash" >&2 || true
    exit 2
  fi
  echo "xberg-bats matches ${ref}"
  exit 0
fi

mkdir -p "$(dirname -- "$lib_file")"
cp "${work}/load.bash" "$lib_file"
digest="$(sha256_of "$lib_file")"
{
  sed -n '/^#/p' "$pin_file"
  printf 'repo=%s\n' "$(pin_value repo)"
  printf 'requested=%s\n' "${requested:-$(pin_value requested)}"
  printf 'ref=%s\n' "$commit"
  printf 'sha256=%s\n' "$digest"
} >"${work}/pin"
cp "${work}/pin" "$pin_file"
echo "xberg-bats synced to ${commit}"
