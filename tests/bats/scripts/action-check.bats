#!/usr/bin/env bats
# scripts/action/check.sh: the governance gate behind the check sub-action (check/action.yml).
#
# git runs for real in a scratch repository; ai-rulez is a stub that records the directory it ran
# in and its arguments, fails on one chosen command line and can write a file when it generates.

setup() {
  bats_load_library xberg-bats
  load "${BATS_TEST_DIRNAME}/../helpers/common"
  xberg_setup
  isolate_home
  SCRIPT="$(repo_root)/scripts/action/check.sh"
  REPO="$BATS_TEST_TMPDIR/repo"
  mkdir -p "$REPO"
  REPO="$(cd "$REPO" && pwd -P)"
  export REPO
  export GIT_AUTHOR_NAME=bats GIT_AUTHOR_EMAIL=bats@example.com
  export GIT_COMMITTER_NAME=bats GIT_COMMITTER_EMAIL=bats@example.com
  unset INPUT_CONFIG_DIRECTORIES INPUT_EXPECTED_CONFIG_COUNT INPUT_PLUGIN INPUT_STRICT INPUT_LOCAL \
    INPUT_TOKEN_BUDGET STUB_FAIL STUB_WRITE
  xberg_stub ai-rulez \
    'here="$(pwd -P)"; here="${here#"$REPO"}"' \
    'printf "%s: %s\n" "${here:-/}" "$*" >>"$XBERG_TRACE"' \
    'if [ -n "${STUB_FAIL:-}" ] && [ "${here:-/}: $*" = "$STUB_FAIL" ]; then exit 2; fi' \
    'if [ -n "${STUB_WRITE:-}" ] && [ "$1" = generate ] && [ "$2" != --check ]; then' \
    '  printf "generated\n" >>"$REPO/$STUB_WRITE"' \
    'fi' \
    'exit 0'
  git -C "$REPO" init -q
}

# Write a configuration file (and optional extra lines) below the scratch repository.
config() {
  mkdir -p "$REPO/$(dirname "$1")"
  printf 'version = "5.0"\n%s' "${2:-}" >"$REPO/$1"
}

commit_all() {
  git -C "$REPO" add -A
  git -C "$REPO" commit -q -m fixture
}

run_check() {
  cd "$REPO" || return 1
  run bash "$SCRIPT"
}

@test "discovers tracked roots in order and runs every step with the strict defaults" {
  config .ai-rulez/config.toml
  printf 'lock\n' >"$REPO/.ai-rulez/ai-rulez.lock"
  config mod/.ai-rulez/config.toml
  config ag/.agents/ai-rulez.toml
  commit_all
  config untracked/.ai-rulez/config.toml

  run_check

  xberg_assert_status 0
  xberg_assert_trace \
    "/: migrate okf --check" \
    "/: validate --strict --no-local" \
    "/: lock --check" \
    "/: generate --strict-config --no-local" \
    "/: generate --check --strict-config --no-local" \
    "/ag: validate --strict --no-local" \
    "/ag: generate --strict-config --no-local" \
    "/ag: generate --check --strict-config --no-local" \
    "/mod: migrate okf --check" \
    "/mod: validate --strict --no-local" \
    "/mod: generate --strict-config --no-local" \
    "/mod: generate --check --strict-config --no-local"
  xberg_assert_github_output $'roots=3\nfiles-examined=4'
  grep -qx 'Result: \*\*passed\*\*' "$GITHUB_STEP_SUMMARY"
  grep -qx -- '- Roots checked: 3' "$GITHUB_STEP_SUMMARY"
  grep -qx -- '- Tracked files examined: 4' "$GITHUB_STEP_SUMMARY"
  grep -qF '| `ag` | `ai-rulez migrate okf --check` | skipped: source = "agents" |' "$GITHUB_STEP_SUMMARY"
  grep -qF '| `mod` | `ai-rulez lock --check` | skipped: no ai-rulez.lock |' "$GITHUB_STEP_SUMMARY"
  grep -qF '| `.` | `git diff --exit-code HEAD` | passed |' "$GITHUB_STEP_SUMMARY"
}

@test "explicit directories are normalized, deduplicated and the optional steps run" {
  config .ai-rulez/config.toml 'source = "agents"'
  config mod/.config/ai-rulez/config.toml
  printf 'lock\n' >"$REPO/mod/.config/ai-rulez/ai-rulez.lock"
  config other/.ai-rulez/config.toml
  commit_all
  export INPUT_CONFIG_DIRECTORIES=$' ./mod/.config/ai-rulez/\r\n\n.ai-rulez\nmod\n'
  export INPUT_STRICT=false INPUT_LOCAL=true INPUT_PLUGIN=true INPUT_TOKEN_BUDGET=20000
  export INPUT_EXPECTED_CONFIG_COUNT=2

  run_check

  xberg_assert_status 0
  xberg_assert_trace \
    "/: validate" \
    "/: generate" \
    "/: generate --check" \
    "/: verify --plugin" \
    "/: tokens --budget 20000" \
    "/mod: migrate okf --check" \
    "/mod: validate" \
    "/mod: lock --check" \
    "/mod: generate" \
    "/mod: generate --check" \
    "/mod: verify --plugin" \
    "/mod: tokens --budget 20000"
}

@test "a configuration count other than the expected one fails before any command runs" {
  config .ai-rulez/config.toml
  config mod/.ai-rulez/config.toml
  commit_all
  export INPUT_EXPECTED_CONFIG_COUNT=3

  run_check

  xberg_assert_status 1
  xberg_assert_output "::error::expected 3 configuration directories but found 2: . mod"
  xberg_assert_trace_empty
}

@test "invalid inputs are rejected before any command runs" {
  config .ai-rulez/config.toml
  mkdir -p "$REPO/empty"
  : >"$REPO/empty/.keep"
  commit_all
  local cases=(
    "INPUT_STRICT=yes|strict must be true or false"
    "INPUT_LOCAL=False|local must be true or false"
    "INPUT_PLUGIN=1|plugin must be true or false"
    "INPUT_EXPECTED_CONFIG_COUNT=0|expected-config-count must be a positive integer"
    "INPUT_EXPECTED_CONFIG_COUNT=2;x|expected-config-count must be a positive integer"
    "INPUT_TOKEN_BUDGET=-5|token-budget must be a positive integer"
    "INPUT_CONFIG_DIRECTORIES=/etc|config-directories entries must be relative paths"
    "INPUT_CONFIG_DIRECTORIES=mod/../..|config-directories entries must not contain .."
    "INPUT_CONFIG_DIRECTORIES=missing|config-directories: missing is not a directory"
    "INPUT_CONFIG_DIRECTORIES=empty|config-directories: no ai-rulez configuration in empty"
  )
  local case assignment message
  for case in "${cases[@]}"; do
    assignment="${case%%|*}"
    message="${case#*|}"
    cd "$REPO" || return 1
    run env "$assignment" bash "$SCRIPT"
    xberg_assert_status 1
    xberg_assert_output "::error::$message"
  done
  xberg_assert_trace_empty
}

@test "the first failing command stops the gate and the summary names it" {
  config .ai-rulez/config.toml
  config mod/.ai-rulez/config.toml
  commit_all
  export STUB_FAIL="/: validate --strict --no-local"

  run_check

  xberg_assert_status 1
  xberg_assert_output_contains "::error::[.] ai-rulez validate --strict --no-local failed with exit 2"
  xberg_assert_trace "/: migrate okf --check" "/: validate --strict --no-local"
  grep -qF 'Result: **failed** at `[.] ai-rulez validate --strict --no-local`' "$GITHUB_STEP_SUMMARY"
  grep -qF '| `.` | `ai-rulez validate --strict --no-local` | failed (exit 2) |' "$GITHUB_STEP_SUMMARY"
  [ ! -s "$GITHUB_OUTPUT" ]
}

@test "generation that changes a tracked file fails the clean-checkout check" {
  config .ai-rulez/config.toml
  printf 'committed\n' >"$REPO/CLAUDE.md"
  commit_all
  export STUB_WRITE=CLAUDE.md

  run_check

  xberg_assert_status 1
  xberg_assert_output_contains "::error::generation changed tracked files"
  grep -qF '| `.` | `git diff --exit-code HEAD` | failed |' "$GITHUB_STEP_SUMMARY"
}

@test "generation that leaves a new untracked file fails; an earlier untracked file does not" {
  config .ai-rulez/config.toml
  commit_all
  printf 'before\n' >"$REPO/scratch.txt"

  run_check
  xberg_assert_status 0

  export STUB_WRITE=new-output.md
  run_check

  xberg_assert_status 1
  xberg_assert_output_contains $'new-output.md\n::error::generation left untracked files'
}

@test "a checkout with no tracked files cannot pass" {
  git -C "$REPO" commit -q --allow-empty -m empty
  config .ai-rulez/config.toml
  export INPUT_CONFIG_DIRECTORIES=.ai-rulez

  run_check

  xberg_assert_status 1
  xberg_assert_output "::error::no tracked files to examine; an empty checkout cannot pass"
  xberg_assert_trace_empty
}

@test "no configuration at all fails with a pointer to config-directories" {
  printf 'readme\n' >"$REPO/README.md"
  commit_all

  run_check

  xberg_assert_status 1
  xberg_assert_output "::error::no ai-rulez configuration found; set config-directories or commit one"
}
