#!/usr/bin/env bats
# scripts/action/resolve-version.sh: the version input shared by the root action and its check and
# update sub-actions. curl is a stub that reports the URL GitHub's /releases/latest redirects to.

setup() {
  bats_load_library xberg-bats
  load "${BATS_TEST_DIRNAME}/../helpers/common"
  xberg_setup
  SCRIPT="$(repo_root)/scripts/action/resolve-version.sh"
  unset INPUT_VERSION FAKE_LATEST
  xberg_stub curl \
    'printf "curl %s\n" "$*" >>"$XBERG_TRACE"' \
    '[ -n "${FAKE_LATEST:-}" ] || exit 22' \
    'printf "%s" "$FAKE_LATEST"'
}

@test "explicit versions gain the v prefix without a network call" {
  local input expected
  for input in 5.7.0 v5.7.0 5.8.0-rc.1; do
    : >"$GITHUB_OUTPUT"
    expected="v${input#v}"
    INPUT_VERSION="$input" run bash "$SCRIPT"
    xberg_assert_status 0
    xberg_assert_github_output "tag=$expected"
  done
  xberg_assert_trace_empty
}

@test "latest and an empty input follow the releases/latest redirect" {
  export FAKE_LATEST="https://github.com/Goldziher/ai-rulez/releases/tag/v5.7.0"
  for input in latest ""; do
    : >"$GITHUB_OUTPUT"
    INPUT_VERSION="$input" run bash "$SCRIPT"
    xberg_assert_status 0
    xberg_assert_github_output "tag=v5.7.0"
  done
}

@test "an unresolvable latest release fails" {
  INPUT_VERSION=latest run bash "$SCRIPT"

  xberg_assert_status 1
  xberg_assert_output "::error::could not resolve the latest ai-rulez release"
  [ ! -s "$GITHUB_OUTPUT" ]
}

@test "malformed versions are rejected" {
  local input
  for input in v5 5.7 '5.7.0;id' 5.7.0-beta.1 'v5.7.0
tag=v1.0.0'; do
    INPUT_VERSION="$input" run bash "$SCRIPT"
    xberg_assert_status 1
    xberg_assert_output "::error::version must be X.Y.Z, vX.Y.Z, a release candidate, or latest"
  done
  [ ! -s "$GITHUB_OUTPUT" ]
}
