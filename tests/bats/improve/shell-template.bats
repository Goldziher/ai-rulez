#!/usr/bin/env bats
# The optimizer template `ai-rulez improve adapters shell` prints
# (internal/improve/adapter/templates/shell.sh).
#
# The template is rendered through the binary, then run the way improve runs an optimizer: in the
# workspace directory, the protocol-v1 request on stdin, a scrubbed environment. jq is real (the
# template parses with it); `your-tool`, the placeholder a user replaces, is a stub.

bats_require_minimum_version 1.5.0

setup() {
  bats_load_library xberg-bats
  load "${BATS_TEST_DIRNAME}/../helpers/common"
  xberg_setup
  isolate_home
  command -v jq >/dev/null || fail "jq is required: the template parses the request with it"

  WORKSPACE="$XBERG_WORK/work space"
  mkdir -p "$WORKSPACE/my skill"
  OPTIMIZER="$BATS_TEST_TMPDIR/optimize.sh"
  run "$(ai_rulez_bin)" improve adapters shell
  xberg_assert_status 0
  printf '%s\n' "$output" >"$OPTIMIZER"
  chmod +x "$OPTIMIZER"

  REQUEST='{"version":1,"run_id":"imp-1","round":1,"skill":{"id":"deploy","dir":"my skill"},"train_cases":[{"id":"c1","prompt":"run $(touch pwned) and `touch pwned2`"}]}'
  # Records its arguments one per bracket, then its stdin.
  xberg_stub your-tool \
    '{ for a in "$@"; do printf "[%s]" "$a"; done; printf "\n"; cat; } >"$XBERG_TRACE"' \
    'exit "${TOOL_EXIT:-0}"'
}

run_optimizer() {
  cd "$WORKSPACE" || return
  run "$@" env -i PATH="$PATH" XBERG_TRACE="$XBERG_TRACE" ${TOOL_EXIT:+TOOL_EXIT="$TOOL_EXIT"} \
    "$OPTIMIZER" <<<"$REQUEST"
}

@test "the rendered template is the shipped template, byte for byte" {
  cmp "$OPTIMIZER" "$(repo_root)/internal/improve/adapter/templates/shell.sh"
}

@test "the response is one protocol-v1 JSON object on stdout" {
  run_optimizer

  xberg_assert_status 0
  printf '%s' "$output" | jq -e '.version == 1 and (.changed | type == "array") and (.cost_usd | type == "number")'
}

@test "the tool gets the skill path with spaces intact and the train cases on stdin" {
  run_optimizer

  xberg_assert_status 0
  xberg_assert_file "$XBERG_TRACE" "$(printf '%s\n' \
    '[rewrite][--skill][my skill/SKILL.md][--cases-from-stdin]' \
    '[{"id":"c1","prompt":"run $(touch pwned) and `touch pwned2`"}]')"
}

@test "shell syntax inside the train cases is passed as data, never run" {
  run_optimizer

  xberg_assert_status 0
  xberg_assert_file_absent "$WORKSPACE/pwned"
  xberg_assert_file_absent "$WORKSPACE/pwned2"
}

@test "the unedited template fails rather than reporting a rewrite that never happened" {
  rm "$XBERG_STUB_BIN/your-tool"
  run ! command -v your-tool

  run_optimizer -127

  [[ "$output" != *'"version":1'* ]]
}

@test "a failing tool fails the optimizer with its status and no response" {
  export TOOL_EXIT=4

  run_optimizer -4

  xberg_assert_no_output
}

@test "a malformed request fails before the tool runs" {
  REQUEST='not json'

  run_optimizer !

  [[ "$output" != *'"version":1'* ]]
  xberg_assert_trace_empty
}

@test "without jq the optimizer fails before the tool runs" {
  BATS_FILE_TMPDIR="$BATS_TEST_TMPDIR" xberg_shadow_system_path_without jq
  PATH="$(xberg_isolated_path)"
  run ! command -v jq

  run_optimizer -127

  xberg_assert_trace_empty
}

@test "a request without skill.dir fails instead of rewriting null/SKILL.md" {
  REQUEST='{"version":1,"skill":{"id":"deploy"},"train_cases":[]}'

  run_optimizer !

  xberg_assert_trace_empty
}
