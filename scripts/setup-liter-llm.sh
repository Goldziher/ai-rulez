#!/usr/bin/env bash
# Download, sha256-verify and extract the liter-llm native library for this repo's pinned version,
# so ai-rulez links it STATICALLY.
#
# The Go binding's cgo directives say `-lliter_llm_ffi` and look in a directory that does not exist
# in the module cache. If a directory on the linker path holds both libliter_llm_ffi.a and the
# shared library, the linker takes the shared one and the binary then needs it at runtime. So this
# script extracts ONLY the static library (or, in dll mode on Windows, only the DLL) into an
# otherwise empty directory, and points cgo at it with CGO_LDFLAGS=-L<dir>.
#
# Usage: setup-liter-llm.sh [--platform P] [--mode auto|static|dll] [--format export|github-env|dir]
#   --platform  linux-x86_64 | linux-aarch64 | macos-arm64 | macos-x86_64 | windows-x86_64
#               (default: the host)
#   --mode      auto (default: static, or dll on Windows), static, or dll (Windows only)
#   --format    export      print `export CGO_LDFLAGS=...` (and CGO_ENABLED=1) for `eval`   (default)
#               github-env  append CGO_ENABLED/CGO_LDFLAGS to $GITHUB_ENV
#               dir         print only the library directory
# Env: LITER_LLM_CACHE (default ${XDG_CACHE_HOME:-$HOME/.cache}/ai-rulez/liter-llm),
#      LITER_LLM_VERSION (must equal go.mod's; a mismatch is an error),
#      LITER_LLM_BASE_URL (override the download base, for tests).
# Progress goes to stderr; stdout carries only the requested result.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
platform="" mode="auto" format="export"
while [ $# -gt 0 ]; do
  case "$1" in
  --platform)
    platform="${2:?}"
    shift 2
    ;;
  --mode)
    mode="${2:?}"
    shift 2
    ;;
  --format)
    format="${2:?}"
    shift 2
    ;;
  -h | --help)
    sed -n '2,21p' "${BASH_SOURCE[0]}" >&2
    exit 0
    ;;
  *)
    echo "unknown argument: $1" >&2
    exit 2
    ;;
  esac
done

log() { echo "setup-liter-llm: $*" >&2; }

if [ -z "$platform" ]; then
  case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) platform=linux-x86_64 ;;
  Linux-aarch64 | Linux-arm64) platform=linux-aarch64 ;;
  Darwin-arm64) platform=macos-arm64 ;;
  Darwin-x86_64) platform=macos-x86_64 ;;
  MINGW*-x86_64 | MSYS*-x86_64 | CYGWIN*-x86_64) platform=windows-x86_64 ;;
  *)
    echo "error: unsupported host $(uname -s)-$(uname -m); pass --platform" >&2
    exit 1
    ;;
  esac
fi
case "$platform" in
linux-x86_64 | linux-aarch64 | macos-arm64 | macos-x86_64 | windows-x86_64) ;;
*)
  echo "error: unsupported platform '$platform'" >&2
  exit 1
  ;;
esac
case "$mode" in
auto)
  # The Windows static archive is MSVC-built and cannot link with Go's MinGW
  # cgo, so Windows links the released DLL instead; every other platform links
  # the static archive.
  if [ "$platform" = windows-x86_64 ]; then mode=dll; else mode=static; fi
  ;;
static) ;;
dll)
  [ "$platform" = windows-x86_64 ] || {
    echo "error: --mode dll is Windows only" >&2
    exit 1
  }
  ;;
*)
  echo "error: unknown mode '$mode'" >&2
  exit 2
  ;;
esac

version="$(bash "$here/liter-llm-version.sh")"
if [ -n "${LITER_LLM_VERSION:-}" ] && [ "${LITER_LLM_VERSION#v}" != "$version" ]; then
  echo "error: LITER_LLM_VERSION=$LITER_LLM_VERSION disagrees with go.mod ($version)" >&2
  exit 1
fi

base="${LITER_LLM_BASE_URL:-https://github.com/xberg-io/liter-llm/releases/download/v${version}}"
asset="liter-llm-go-v${version}-${platform}"
cache="${LITER_LLM_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/ai-rulez/liter-llm}"
if [ "$mode" = dll ]; then
  member="lib/liter_llm_ffi.dll"
elif [ "$platform" = windows-x86_64 ]; then
  member="lib/liter_llm_ffi.lib"
else
  member="lib/libliter_llm_ffi.a"
fi
libdir="$cache/v${version}/${platform}-${mode}"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

if [ ! -f "$libdir/.verified" ]; then
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' EXIT
  log "downloading $asset.tar.gz (liter-llm v$version)"
  curl -fsSL --retry 3 --retry-delay 2 -o "$work/$asset.tar.gz" "$base/$asset.tar.gz"
  curl -fsSL --retry 3 --retry-delay 2 -o "$work/$asset.tar.gz.sha256" "$base/$asset.tar.gz.sha256"

  # Sidecar is `<hex>  <name>` or a bare `<hex>`. Fail closed on anything else.
  expected="$(awk 'NR == 1 { print tolower($1) }' "$work/$asset.tar.gz.sha256")"
  case "$expected" in
  *[!0-9a-f]* | "")
    echo "error: unparsable checksum sidecar for $asset" >&2
    exit 1
    ;;
  esac
  [ "${#expected}" -eq 64 ] || {
    echo "error: checksum sidecar for $asset is not a sha256" >&2
    exit 1
  }
  actual="$(sha256_of "$work/$asset.tar.gz")"
  if [ "$actual" != "$expected" ]; then
    echo "error: sha256 mismatch for $asset.tar.gz: expected $expected, got $actual" >&2
    exit 1
  fi
  log "sha256 verified ($actual)"

  # Extract just the one member; never the header, shared library or native-static-libs.txt.
  mkdir -p "$work/x"
  tar -xzf "$work/$asset.tar.gz" -C "$work/x" "$asset/$member"
  rm -rf "$libdir"
  mkdir -p "$libdir"
  mv "$work/x/$asset/$member" "$libdir/"
  printf '%s\n' "$actual" >"$libdir/.verified"
else
  log "using cached $libdir"
fi

# GNU ld on Windows (mingw) wants a native path with forward slashes.
dir="$libdir"
if command -v cygpath >/dev/null 2>&1; then dir="$(cygpath -m "$libdir")"; fi
ldflags="-L$dir"
case "$dir" in *" "*) ldflags="\"-L$dir\"" ;; esac

case "$format" in
dir) printf '%s\n' "$dir" ;;
export)
  printf 'export CGO_ENABLED=1\n'
  printf 'export CGO_LDFLAGS=%q\n' "$ldflags"
  ;;
github-env)
  : "${GITHUB_ENV:?--format github-env needs GITHUB_ENV}"
  {
    echo "CGO_ENABLED=1"
    echo "CGO_LDFLAGS=$ldflags"
  } >>"$GITHUB_ENV"
  if [ "$mode" = dll ]; then
    # Windows has no rpath: the test and build steps find the DLL on PATH.
    : "${GITHUB_PATH:?--format github-env with dll mode needs GITHUB_PATH}"
    native="$libdir"
    if command -v cygpath >/dev/null 2>&1; then native="$(cygpath -w "$libdir")"; fi
    printf '%s\n' "$native" >>"$GITHUB_PATH"
    log "added $native to PATH (Windows DLL)"
  fi
  log "exported CGO_ENABLED=1 CGO_LDFLAGS=$ldflags"
  ;;
*)
  echo "error: unknown format '$format'" >&2
  exit 2
  ;;
esac
