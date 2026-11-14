#!/usr/bin/env bash
# Dry-run the REAL release packaging path for the host platform and check what the npm, PyPI and
# Homebrew wrappers rely on: the archive name, the files at its root, a checksums.txt line in the
# format they parse, a binary that runs and reports the version, and no runtime dependency on the
# liter-llm shared library (the binary must be statically linked). It then wraps that archive into the
# host's PyPI wheel and npm platform package, installs both into clean environments with the GitHub
# downloader unreachable, and runs the installed binary.
#
# Usage: scripts/release-dry-run.sh [version]    (default: 0.0.0-rc.0)
# Output goes to a temporary directory that is removed on exit.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

VERSION="${1:-0.0.0-rc.0}"
VERSION="${VERSION#v}"
goos="$(go env GOHOSTOS)"
goarch="$(go env GOHOSTARCH)"
ext="tar.gz"
exe=""
if [ "$goos" = windows ]; then
  ext="zip"
  exe=".exe"
fi
name="ai-rulez_${VERSION}_${goos}_${goarch}.${ext}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fail() {
  echo "dry-run FAILED: $*" >&2
  exit 1
}

echo "==> liter-llm pin: $(bash scripts/liter-llm-version.sh)"
echo "==> packaging ${goos}/${goarch} via scripts/package-release.sh"
VERSION="$VERSION" OUT_DIR="$work/dist" bash scripts/package-release.sh "${goos}/${goarch}"
[ -f "$work/dist/$name" ] || fail "expected $name was not produced"

echo "==> reproducibility (second build must be byte-identical)"
VERSION="$VERSION" OUT_DIR="$work/dist2" bash scripts/package-release.sh "${goos}/${goarch}" >/dev/null
if command -v sha256sum >/dev/null 2>&1; then
  sum() { sha256sum "$1" | awk '{print $1}'; }
else
  sum() { shasum -a 256 "$1" | awk '{print $1}'; }
fi
[ "$(sum "$work/dist/$name")" = "$(sum "$work/dist2/$name")" ] ||
  echo "warning: archive differs between two builds (non-deterministic input, e.g. the toolchain); not fatal" >&2

echo "==> checksums.txt in the format release/npm and release/pypi parse"
(cd "$work/dist" && printf '%s  %s\n' "$(sum "$name")" "$name" >checksums.txt)
awk -v n="$name" '{p=$0; gsub(/^[ \t]+|[ \t]+$/, "", p); split(p, a, /[ \t]+/); if (a[2]==n && length(a[1])==64) ok=1} END{exit !ok}' \
  "$work/dist/checksums.txt" || fail "checksums.txt line not parseable as '<sha256>  <archive>'"

echo "==> archive layout"
mkdir -p "$work/x"
if [ "$ext" = zip ]; then
  py=python3
  command -v python3 >/dev/null 2>&1 || py=python
  "$py" -I -c 'import sys,zipfile; zipfile.ZipFile(sys.argv[1]).extractall(sys.argv[2])' "$work/dist/$name" "$work/x"
else
  tar -xzf "$work/dist/$name" -C "$work/x"
fi
for f in "ai-rulez${exe}" README.md LICENSE THIRD_PARTY_NOTICES.md LICENSES/*; do
  [ -f "$work/x/$f" ] || fail "$f missing from the archive"
done

bin="$work/x/ai-rulez${exe}"
echo "==> binary runs and reports ${VERSION}"
"$bin" version | tr -d '\r' | grep -qF "$VERSION" || fail "'ai-rulez version' does not contain ${VERSION}"
"$bin" llm doctor >/dev/null || fail "'ai-rulez llm doctor' failed"

echo "==> no runtime dependency on the liter-llm shared library"
case "$goos" in
darwin) deps="$(otool -L "$bin")" ;;
linux) deps="$(ldd "$bin" 2>&1 || true)" ;;
*) deps="$(ls "$work/x")" ;;
esac
if printf '%s' "$deps" | grep -qi 'liter_llm_ffi'; then
  if [ "$goos" = windows ]; then
    echo "note: Windows archive ships liter_llm_ffi.dll (dll mode); npm/PyPI wrappers install it beside the exe"
  else
    fail "binary depends on the shared liter_llm_ffi library (linked dynamically):
$deps"
  fi
fi

echo "==> Homebrew formula renders from these checksums (darwin/linux archives only)"
if [ "$goos" != windows ]; then
  : # the formula needs all four unix archives; render with this host's checksum standing in for each
  fake="$work/fake-checksums.txt"
  : >"$fake"
  for t in darwin_arm64 darwin_amd64 linux_arm64 linux_amd64; do
    printf '%s  ai-rulez_%s_%s.tar.gz\n' "$(sum "$work/dist/$name")" "$VERSION" "$t" >>"$fake"
  done
  bash scripts/update-homebrew-formula.sh "$VERSION" "$fake" | grep -q 'class AiRulez < Formula' ||
    fail "formula did not render"
fi

echo "==> registry packages install and run with no GitHub access"
py=python3
command -v python3 >/dev/null 2>&1 || py=python
command -v npm >/dev/null 2>&1 || fail "npm is required to rehearse the npm platform package"
only="${goos}_${goarch}"
node_os="$goos"
[ "$goos" = windows ] && node_os=win32
node_cpu="x64"
[ "$goarch" = arm64 ] && node_cpu="arm64"

"$py" -I scripts/release/package_wheels.py --archives "$work/dist" --version "$VERSION" --out "$work/wheels" --only "$only" >/dev/null
"$py" -I scripts/release/package_npm.py --archives "$work/dist" --version "$VERSION" --out "$work/npm" --only "$only" >/dev/null

offline() {
  HOME="$work/empty-home" USERPROFILE="$work/empty-home" HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9 "$@"
}
mkdir -p "$work/empty-home"

"$py" -m venv "$work/venv"
venv_bin="$work/venv/bin"
[ "$goos" = windows ] && venv_bin="$work/venv/Scripts"
"$venv_bin/python" -m pip install --quiet --no-index "$work"/wheels/ai_rulez-*.whl
offline "$venv_bin/ai-rulez" --version | tr -d '\r' | grep -qF "$VERSION" ||
  fail "the installed wheel does not run ai-rulez ${VERSION} without GitHub access"

mkdir -p "$work/rootpkg" "$work/tarballs" "$work/project"
cp release/npm/ai-rulez.js release/npm/install.js "$work/rootpkg/"
cp "$work/npm/root/package.json" "$work/rootpkg/package.json"
(cd "$work/npm/${node_os}-${node_cpu}" && npm pack --silent --pack-destination "$work/tarballs" >/dev/null)
(cd "$work/rootpkg" && npm pack --silent --pack-destination "$work/tarballs" >/dev/null)
(cd "$work/project" && npm init -y >/dev/null && offline npm install --silent --omit=optional --no-audit --no-fund \
  "$work"/tarballs/ai-rulez-"${node_os}"-"${node_cpu}"-*.tgz "$work"/tarballs/ai-rulez-[0-9]*.tgz)
offline node "$work/project/node_modules/ai-rulez/ai-rulez.js" --version | tr -d '\r' | grep -qF "$VERSION" ||
  fail "the installed npm package does not run ai-rulez ${VERSION} without GitHub access"
[ ! -e "$work/project/node_modules/ai-rulez/bin" ] ||
  fail "the npm launcher downloaded a binary instead of using the platform package"

echo "OK: ${name} packaged and verified (ai-rulez ${VERSION}, ${goos}/${goarch})"
