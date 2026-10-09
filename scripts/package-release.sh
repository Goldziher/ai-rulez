#!/usr/bin/env bash
# Build and package ai-rulez for the HOST platform.
#
# ai-rulez links the liter-llm native library through cgo, so a release binary can only be built
# natively on its own platform (no cross-compiling from one runner). This script:
#   1. fetches + sha256-verifies the liter-llm static library pinned by go.mod (setup-liter-llm.sh),
#   2. builds with CGO_ENABLED=1, statically linking it,
#   3. smoke-tests the binary,
#   4. writes ai-rulez_<version>_<os>_<arch>.{tar.gz,zip} (README.md, LICENSE, the binary at the
#      archive root: the layout release/npm and release/pypi extract).
#
# Usage: package-release.sh <os>/<arch>      e.g. linux/amd64, darwin/arm64, windows/amd64
# Env:   VERSION        release version without "v" (required; becomes main.version)
#        OUT_DIR        where the archive goes (default: dist)
#        WINDOWS_LINK   auto (default) | static | dll   Windows only, see docs/maintainers/release.md
#        SKIP_SMOKE=1   do not run the binary (never set in CI)
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "Usage: $0 <os>/<arch>" >&2
  exit 2
fi
target="$1"
: "${VERSION:?VERSION is required (e.g. 5.0.0, no leading v)}"
VERSION="${VERSION#v}"
OUT_DIR="${OUT_DIR:-dist}"
WINDOWS_LINK="${WINDOWS_LINK:-auto}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

goos="${target%/*}"
goarch="${target#*/}"
case "$goos/$goarch" in
linux/amd64) native=linux-x86_64 ;;
linux/arm64) native=linux-aarch64 ;;
darwin/arm64) native=macos-arm64 ;;
darwin/amd64) native=macos-x86_64 ;;
windows/amd64) native=windows-x86_64 ;;
*)
  echo "error: unsupported target '$target' (windows/arm64 is not supported)" >&2
  exit 1
  ;;
esac

if [ "$(go env GOHOSTOS)/$(go env GOHOSTARCH)" != "$target" ]; then
  echo "error: $target cannot be cross-compiled (cgo + native static library); host is $(go env GOHOSTOS)/$(go env GOHOSTARCH)" >&2
  exit 1
fi

exe=""
archive_ext="tar.gz"
if [ "$goos" = windows ]; then
  exe=".exe"
  archive_ext="zip"
fi
archive="ai-rulez_${VERSION}_${goos}_${goarch}.${archive_ext}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
stage="$work/stage"
mkdir -p "$stage" "$OUT_DIR"

smoke() {
  local bin="$1"
  [ "${SKIP_SMOKE:-}" = 1 ] && return 0
  local got
  got="$("$bin" version 2>&1 | tr -d '\r')" || {
    echo "smoke: '$bin version' failed: $got" >&2
    return 1
  }
  case "$got" in
  *"$VERSION"*) ;;
  *)
    echo "smoke: '$bin version' printed '$got', expected it to contain $VERSION" >&2
    return 1
    ;;
  esac
  "$bin" llm doctor >/dev/null || {
    echo "smoke: '$bin llm doctor' failed (liter-llm not linked/loadable)" >&2
    return 1
  }
}

# build <mode>: static link (default everywhere) or Windows dll. Leaves the binary in $stage.
build() {
  local mode="$1" libdir
  libdir="$(bash scripts/setup-liter-llm.sh --platform "$native" --mode "$mode" --format dir)"
  rm -rf "$stage"
  mkdir -p "$stage"
  CGO_ENABLED=1 CGO_LDFLAGS="-L$libdir" go build \
    -trimpath -buildvcs=false \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o "$stage/ai-rulez${exe}" ./cmd/ai-rulez
  if [ "$mode" = dll ]; then
    cp "$libdir/liter_llm_ffi.dll" "$stage/"
  fi
  smoke "$stage/ai-rulez${exe}"
}

link_mode=static
if [ "$goos" = windows ]; then
  case "$WINDOWS_LINK" in
  static) build static ;;
  dll)
    link_mode=dll
    build dll
    ;;
  auto)
    # The native Windows library is an MSVC-built static archive; the Go toolchain's cgo uses
    # mingw gcc. Linking it is unverified, so a failed static build (link or smoke) falls back to
    # shipping the DLL next to ai-rulez.exe.
    if ! build static; then
      echo "warning: static Windows link failed; falling back to liter_llm_ffi.dll beside ai-rulez.exe" >&2
      link_mode=dll
      build dll
    fi
    ;;
  *)
    echo "error: WINDOWS_LINK must be auto|static|dll" >&2
    exit 2
    ;;
  esac
else
  build static
fi

cp README.md LICENSE "$stage/"

# Deterministic archives: fixed mtime, sorted names, no owner info, no gzip timestamp.
files=()
while IFS= read -r f; do files+=("$f"); done < <(cd "$stage" && find . -maxdepth 1 -type f -print | sed 's|^\./||' | LC_ALL=C sort)
(cd "$stage" && TZ=UTC touch -t 198001010000 "${files[@]}")

out="$(cd "$OUT_DIR" && pwd)/$archive"
rm -f "$out"
if [ "$archive_ext" = tar.gz ]; then
  if tar --version 2>/dev/null | grep -q 'GNU tar'; then
    tar_flags=(--sort=name --owner=0 --group=0 --numeric-owner --mtime='1980-01-01 00:00:00 UTC')
  else
    tar_flags=(--uid 0 --gid 0 --uname '' --gname '')
  fi
  (cd "$stage" && tar "${tar_flags[@]}" -cf - "${files[@]}") | gzip -n -9 >"$out"
else
  py=python3
  command -v python3 >/dev/null 2>&1 || py=python
  "$py" -I - "$stage" "$out" "${files[@]}" <<'PY'
import sys, zipfile
stage, out, *names = sys.argv[1:]
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
    for name in names:
        info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
        info.compress_type = zipfile.ZIP_DEFLATED
        info.external_attr = (0o755 if name.endswith(".exe") else 0o644) << 16
        with open(f"{stage}/{name}", "rb") as f:
            z.writestr(info, f.read())
PY
fi

echo "packaged $out ($target, liter-llm linked: $link_mode)"
