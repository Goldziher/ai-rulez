#!/usr/bin/env bats
# scripts/pre-commit/run-ai-rulez.sh: the pre-commit entry point that finds or downloads ai-rulez.
#
# The script PROBES for ai-rulez with `command -v`, so PATH is a shadowed mirror of the host
# without it (a developer machine usually has one installed). curl and uname are stubs: curl
# serves a fake release built in setup, so tar, unzip and the checksum tools run for real.

setup_file() {
  bats_load_library xberg-bats
  xberg_shadow_system_path_without ai-rulez
}

setup() {
  bats_load_library xberg-bats
  load "${BATS_TEST_DIRNAME}/../helpers/common"
  xberg_setup_isolated
  isolate_home
  export PATH
  PATH="$(xberg_isolated_path)"

  SCRIPT="$(repo_root)/scripts/pre-commit/run-ai-rulez.sh"
  RELEASE="$BATS_TEST_TMPDIR/release"
  export RELEASE
  export AI_RULEZ_CACHE_DIR="$BATS_TEST_TMPDIR/cache dir"
  export AI_RULEZ_VERSION="v9.8.7"
  export TMPDIR="$BATS_TEST_TMPDIR/tmp"
  mkdir -p "$RELEASE" "$TMPDIR"
  unset AI_RULEZ_BINARY FAKE_EXIT

  export FAKE_UNAME_S="Darwin" FAKE_UNAME_M="arm64"
  xberg_stub uname \
    'case "$1" in -s) echo "$FAKE_UNAME_S" ;; -m) echo "$FAKE_UNAME_M" ;; *) exit 2 ;; esac'
  stub_release_curl
}

# A curl that serves files from $RELEASE by URL basename and records each URL, or fails like
# `curl -f` on a 404 when the release has no such asset.
stub_release_curl() {
  xberg_stub curl \
    'url="" target=""' \
    'while [ $# -gt 0 ]; do' \
    '  case "$1" in -o) target="$2"; shift ;; https://*) url="$1" ;; esac' \
    '  shift' \
    'done' \
    'printf "curl %s\n" "$url" >>"$XBERG_TRACE"' \
    'asset="$RELEASE/${url##*/}"' \
    '[ -f "$asset" ] || { echo "curl: (22) The requested URL returned error: 404" >&2; exit 22; }' \
    'cp "$asset" "$target"'
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# Build ai-rulez_9.8.7_<platform>_<arch>.<ext> holding a fake binary that echoes its arguments
# one per bracket, so a split or re-quoted argument shows. Pass "nobinary" to leave it out.
make_release() {
  local platform="$1" arch="$2" ext="${3:-tar.gz}" contents="${4:-binary}"
  local name="ai-rulez_9.8.7_${platform}_${arch}.${ext}" stage="$BATS_TEST_TMPDIR/stage"
  local exe="ai-rulez"
  [ "$platform" != windows ] || exe="ai-rulez.exe"
  rm -rf "$stage"
  mkdir -p "$stage"
  printf 'readme\n' >"$stage/README.md"
  if [ "$contents" = binary ]; then
    printf '%s\n' '#!/bin/sh' \
      'printf "fake ai-rulez:"; for a in "$@"; do printf " [%s]" "$a"; done; printf "\n"' \
      'exit "${FAKE_EXIT:-0}"' >"$stage/$exe"
    chmod +x "$stage/$exe"
  fi
  if [ "$ext" = zip ]; then
    (cd "$stage" && zip -q -r "$RELEASE/$name" .)
  else
    tar -czf "$RELEASE/$name" -C "$stage" .
  fi
  ARCHIVE="$name"
}

write_checksums() {
  printf '%s  %s\n' "$(sha256 "$RELEASE/$ARCHIVE")" "$ARCHIVE" >"$RELEASE/checksums.txt"
}

cached_binary() {
  printf '%s/v9.8.7/%s/ai-rulez%s' "$AI_RULEZ_CACHE_DIR" "$1" "${2:-}"
}

@test "the fake ai-rulez and curl resolve before anything on the host" {
  [ "$(command -v curl)" = "$XBERG_STUB_BIN/curl" ]
  [ "$(command -v uname)" = "$XBERG_STUB_BIN/uname" ]
  run command -v ai-rulez
  xberg_assert_status 1
}

@test "AI_RULEZ_BINARY is executed with every argument passed through verbatim" {
  make_release darwin arm64
  cp "$BATS_TEST_TMPDIR/stage/ai-rulez" "$BATS_TEST_TMPDIR/my ai-rulez"
  export AI_RULEZ_BINARY="$BATS_TEST_TMPDIR/my ai-rulez"
  xberg_stub_curl_offline

  run "$SCRIPT" generate --profile "a b" "it's" '$HOME' ""

  xberg_assert_status 0
  xberg_assert_output 'fake ai-rulez: [generate] [--profile] [a b] [it'"'"'s] [$HOME] []'
}

@test "an ai-rulez on PATH is used without downloading" {
  xberg_stub ai-rulez 'printf "path ai-rulez %s\n" "$*"'
  xberg_stub_curl_offline

  run "$SCRIPT" generate --dry-run

  xberg_assert_status 0
  xberg_assert_output "path ai-rulez generate --dry-run"
}

@test "a missing binary is downloaded, checksum-verified, cached and run" {
  make_release darwin arm64
  write_checksums

  run "$SCRIPT" generate

  xberg_assert_status 0
  xberg_assert_lines \
    "Downloading ai-rulez v9.8.7 for darwin/arm64..." \
    "fake ai-rulez: [generate]"
  xberg_assert_trace \
    "curl https://github.com/Goldziher/ai-rulez/releases/download/v9.8.7/ai-rulez_9.8.7_darwin_arm64.tar.gz" \
    "curl https://github.com/Goldziher/ai-rulez/releases/download/v9.8.7/checksums.txt"
  [ -x "$(cached_binary darwin-arm64)" ]
  [ -z "$(ls -A "$TMPDIR")" ]
}

@test "a cached binary is reused without touching the network" {
  make_release darwin arm64
  write_checksums
  run "$SCRIPT" generate
  xberg_assert_status 0
  xberg_stub_curl_offline

  run "$SCRIPT" validate

  xberg_assert_status 0
  xberg_assert_output "fake ai-rulez: [validate]"
}

@test "uname output maps to the release platform and architecture" {
  local case s m want
  for case in "Linux x86_64 linux_amd64" "Linux aarch64 linux_arm64" "Darwin x86_64 darwin_amd64" \
    "Linux amd64 linux_amd64"; do
    read -r s m want <<<"$case"
    export FAKE_UNAME_S="$s" FAKE_UNAME_M="$m"
    make_release "${want%_*}" "${want#*_}"
    write_checksums
    rm -rf "$XBERG_TRACE" "$AI_RULEZ_CACHE_DIR"

    run "$SCRIPT" version

    xberg_assert_status 0
    xberg_assert_output_contains "fake ai-rulez: [version]"
    grep -q "ai-rulez_9.8.7_${want}.tar.gz" "$XBERG_TRACE"
  done
}

@test "an unsupported platform fails before any download" {
  export FAKE_UNAME_S="SunOS"
  xberg_stub_curl_offline

  run "$SCRIPT" generate

  xberg_assert_status 1
  xberg_assert_output "Unsupported platform: SunOS"
}

@test "an unsupported architecture fails before any download" {
  export FAKE_UNAME_M="riscv64"
  xberg_stub_curl_offline

  run "$SCRIPT" generate

  xberg_assert_status 1
  xberg_assert_output "Unsupported architecture: riscv64"
}

@test "a checksum mismatch fails and caches nothing" {
  make_release darwin arm64
  printf '%s  %s\n' "0000000000000000000000000000000000000000000000000000000000000000" "$ARCHIVE" \
    >"$RELEASE/checksums.txt"

  run "$SCRIPT" generate

  xberg_assert_status 1
  xberg_assert_output_contains "Checksum verification failed for ai-rulez_9.8.7_darwin_arm64.tar.gz"
  xberg_assert_file_absent "$(cached_binary darwin-arm64)"
  [ -z "$(ls -A "$TMPDIR")" ]
}

@test "without shasum or sha256sum the checksum is skipped with a warning" {
  BATS_FILE_TMPDIR="$BATS_TEST_TMPDIR" xberg_shadow_system_path_without ai-rulez shasum sha256sum
  PATH="$(xberg_isolated_path)"
  make_release darwin arm64
  # make_release ran before PATH lost the tools; the digest is irrelevant once they are gone.
  printf 'deadbeef  %s\n' "$ARCHIVE" >"$RELEASE/checksums.txt"

  run "$SCRIPT" generate

  xberg_assert_status 0
  xberg_assert_output_contains "Neither shasum nor sha256sum is available, skipping checksum verification"
  xberg_assert_output_contains "fake ai-rulez: [generate]"
}

@test "the binary's exit status is the script's exit status" {
  make_release darwin arm64
  write_checksums
  export FAKE_EXIT=3

  run "$SCRIPT" lock

  xberg_assert_status 3
}

@test "a failed download exits non-zero and leaves no cache entry or temp dir" {
  run "$SCRIPT" generate

  [ "$status" -ne 0 ]
  xberg_assert_output_contains "404"
  xberg_assert_file_absent "$(cached_binary darwin-arm64)"
  [ -z "$(ls -A "$TMPDIR")" ]
}

@test "an archive without the binary fails and caches nothing" {
  make_release darwin arm64 tar.gz nobinary
  write_checksums

  run "$SCRIPT" generate

  [ "$status" -ne 0 ]
  xberg_assert_file_absent "$(cached_binary darwin-arm64)"
  [ -z "$(ls -A "$TMPDIR")" ]
}

@test "windows downloads the zip and runs ai-rulez.exe" {
  export FAKE_UNAME_S="MINGW64_NT-10.0-19045" FAKE_UNAME_M="x86_64"
  make_release windows amd64 zip
  write_checksums

  run "$SCRIPT" generate

  xberg_assert_status 0
  xberg_assert_output_contains "fake ai-rulez: [generate]"
  [ -x "$(cached_binary windows-amd64 .exe)" ]
}

@test "windows without unzip fails with a clear message" {
  BATS_FILE_TMPDIR="$BATS_TEST_TMPDIR" xberg_shadow_system_path_without ai-rulez unzip
  export FAKE_UNAME_S="MSYS_NT-10.0" FAKE_UNAME_M="x86_64"
  make_release windows amd64 zip
  write_checksums
  PATH="$(xberg_isolated_path)"

  run "$SCRIPT" generate

  xberg_assert_status 1
  xberg_assert_output_contains "unzip is required to extract ai-rulez"
  xberg_assert_file_absent "$(cached_binary windows-amd64 .exe)"
}
