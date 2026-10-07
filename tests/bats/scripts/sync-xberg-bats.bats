#!/usr/bin/env bats
# scripts/bats/sync-xberg-bats.sh: re-pins and drift-checks the vendored xberg-bats library.
#
# Each test copies the script into a scratch consumer tree (its root is found from the script's
# own location) and points XBERG_ACTIONS_URL at a local git repository standing in for
# xberg-io/actions, so nothing touches the network or this checkout.

setup() {
  bats_load_library xberg-bats
  load "${BATS_TEST_DIRNAME}/../helpers/common"
  xberg_setup
  isolate_home
  export GIT_AUTHOR_NAME=bats GIT_AUTHOR_EMAIL=bats@example.com
  export GIT_COMMITTER_NAME=bats GIT_COMMITTER_EMAIL=bats@example.com

  UPSTREAM="$XBERG_WORK/actions"
  CONSUMER="$XBERG_WORK/consumer with space"
  export XBERG_ACTIONS_URL="$UPSTREAM"

  mkdir -p "$UPSTREAM/bats-lib/xberg-bats" "$CONSUMER/scripts/bats" "$CONSUMER/tests/lib/xberg-bats"
  git -C "$UPSTREAM" init -q
  printf 'helpers v1\n' >"$UPSTREAM/bats-lib/xberg-bats/load.bash"
  git -C "$UPSTREAM" add -A
  git -C "$UPSTREAM" commit -q -m v1
  git -C "$UPSTREAM" tag -a v1 -m v1
  V1="$(git -C "$UPSTREAM" rev-parse HEAD)"
  printf 'helpers v2\n' >"$UPSTREAM/bats-lib/xberg-bats/load.bash"
  git -C "$UPSTREAM" commit -q -am v2
  V2="$(git -C "$UPSTREAM" rev-parse HEAD)"

  SCRIPT="$CONSUMER/scripts/bats/sync-xberg-bats.sh"
  cp "$(repo_root)/scripts/bats/sync-xberg-bats.sh" "$SCRIPT"
  LIB="$CONSUMER/tests/lib/xberg-bats/load.bash"
  PIN="$CONSUMER/tests/lib/xberg-bats.pin"
  printf 'helpers v1\n' >"$LIB"
  write_pin "$V1" "$(digest "$LIB")"
}

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

write_pin() {
  printf '%s\n' '# pin comment' 'repo=https://github.com/xberg-io/actions.git' 'requested=v1' \
    "ref=$1" "sha256=$2" >"$PIN"
}

@test "--check passes when the copy matches the pinned commit" {
  run "$SCRIPT" --check

  xberg_assert_status 0
  xberg_assert_output_contains "xberg-bats matches $V1"
}

@test "--check exits 2 on a hand edit without needing the network" {
  printf 'helpers v1 edited\n' >"$LIB"
  export XBERG_ACTIONS_URL="$XBERG_WORK/does-not-exist"

  run "$SCRIPT" --check

  xberg_assert_status 2
  xberg_assert_output_contains "was edited by hand"
}

@test "--check exits 2 when the pinned commit differs from the copy" {
  write_pin "$V2" "$(digest "$LIB")"

  run "$SCRIPT" --check

  xberg_assert_status 2
  xberg_assert_output_contains "differs from bats-lib/xberg-bats/load.bash at $V2"
  xberg_assert_output_contains "+helpers v2"
}

@test "--check exits 2 when the copy is missing" {
  rm "$LIB"

  run "$SCRIPT" --check

  xberg_assert_status 2
  xberg_assert_output_contains "is missing"
}

@test "sync with --ref pins the peeled commit of an annotated tag, not the tag object" {
  printf 'stale\n' >"$LIB"

  run "$SCRIPT" --ref v1

  xberg_assert_status 0
  xberg_assert_output_contains "xberg-bats synced to $V1"
  xberg_assert_file "$LIB" "helpers v1"
  xberg_assert_file "$PIN" "$(printf '%s\n' '# pin comment' 'repo=https://github.com/xberg-io/actions.git' \
    'requested=v1' "ref=$V1" "sha256=$(digest "$LIB")")"
  run "$SCRIPT" --check
  xberg_assert_status 0
}

@test "sync without --ref restores the pinned commit and keeps the requested ref" {
  write_pin "$V2" "unknown"

  run "$SCRIPT"

  xberg_assert_status 0
  xberg_assert_file "$LIB" "helpers v2"
  grep -qx "requested=v1" "$PIN"
  grep -qx "ref=$V2" "$PIN"
}

@test "an unreachable repository is an error (exit 1), not drift" {
  export XBERG_ACTIONS_URL="$XBERG_WORK/does-not-exist"

  run "$SCRIPT" --check

  xberg_assert_status 1
  xberg_assert_output_contains "could not fetch $V1"
}

@test "a ref without the library is an error and leaves the copy alone" {
  git -C "$UPSTREAM" rm -q -r bats-lib
  git -C "$UPSTREAM" commit -q -m drop

  run "$SCRIPT" --ref "$(git -C "$UPSTREAM" rev-parse HEAD)"

  xberg_assert_status 1
  xberg_assert_output_contains "does not exist at"
  xberg_assert_file "$LIB" "helpers v1"
}

@test "bad arguments exit 1 with a message" {
  local args
  for args in "--bogus" "--ref" "--check --ref v1"; do
    # shellcheck disable=SC2086  # each case is a word list on purpose
    run "$SCRIPT" $args
    xberg_assert_status 1
  done
}

@test "--help prints the usage and exits 0" {
  run "$SCRIPT" --help

  xberg_assert_status 0
  xberg_assert_output_contains "sync-xberg-bats.sh --check"
}
