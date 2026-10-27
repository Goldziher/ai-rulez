#!/usr/bin/env bats
# scripts/action/update.sh: the scheduled pin update behind the update sub-action.
#
# git runs for real against a local bare remote. ai-rulez and gh are stubs: ai-rulez prints a
# canned `lock --outdated` document and changes the lock on `update`; gh records its calls (the
# body file as BODY, copied for inspection) and whether it received a token, and answers
# `pr list` and `pr create` with fixed URLs.

setup() {
  bats_load_library xberg-bats
  load "${BATS_TEST_DIRNAME}/../helpers/common"
  xberg_setup
  isolate_home
  SCRIPT="$(repo_root)/scripts/action/update.sh"
  export GIT_AUTHOR_NAME=bats GIT_AUTHOR_EMAIL=bats@example.com
  export GIT_COMMITTER_NAME=bats GIT_COMMITTER_EMAIL=bats@example.com
  unset INPUT_NAMES INPUT_KIND INPUT_DIRECT INPUT_BRANCH INPUT_TOKEN INPUT_WORKING_DIRECTORY \
    GITHUB_ACTIONS GITHUB_REPOSITORY GITHUB_TOKEN GH_TOKEN STUB_UPDATE_EXIT STUB_NO_CHANGE STUB_OPEN_PR STUB_PIN

  REMOTE="$BATS_TEST_TMPDIR/remote.git"
  WORK="$BATS_TEST_TMPDIR/work"
  export REMOTE WORK
  git init -q --bare "$REMOTE"
  git init -q "$WORK"
  git -C "$WORK" checkout -q -b main
  git -C "$WORK" remote add origin "$REMOTE"
  mkdir -p "$WORK/.ai-rulez"
  printf 'version = "5.0"\n' >"$WORK/.ai-rulez/config.toml"
  printf 'pin = "v1.0.0"\n' >"$WORK/.ai-rulez/ai-rulez.lock"
  git -C "$WORK" add -A
  git -C "$WORK" commit -q -m init
  git -C "$WORK" push -q origin main

  export STUB_OUTDATED="$BATS_TEST_TMPDIR/outdated.json"
  outdated updatable
  xberg_stub ai-rulez \
    'printf "ai-rulez %s\n" "$*" >>"$XBERG_TRACE"' \
    'case "$1" in' \
    '  lock) [ "$2" != --outdated ] || cat "$STUB_OUTDATED" ;;' \
    '  update) [ -z "${STUB_UPDATE_EXIT:-}" ] || exit "$STUB_UPDATE_EXIT"' \
    '    [ -n "${STUB_NO_CHANGE:-}" ] || printf "pin = \"%s\"\n" "${STUB_PIN:-v1.2.0}" >.ai-rulez/ai-rulez.lock ;;' \
    'esac' \
    'exit 0'
  xberg_stub gh \
    'sub="$1 $2" args=""' \
    'while [ $# -gt 0 ]; do' \
    '  case "$1" in' \
    '    --body-file) cp "$2" "$BATS_TEST_TMPDIR/body.md"; args="$args --body-file BODY"; shift ;;' \
    '    *) args="$args $1" ;;' \
    '  esac' \
    '  shift' \
    'done' \
    'printf "gh%s token=%s\n" "$args" "${GH_TOKEN:+set}" >>"$XBERG_TRACE"' \
    'case "$sub" in' \
    '  "pr list") printf "%s\n" "${STUB_OPEN_PR:-}" ;;' \
    '  "pr create") printf "https://github.com/owner/repo/pull/7\n" ;;' \
    'esac'
}

# Write the `lock --outdated --format json` document: one include with the given status.
outdated() {
  cat >"$STUB_OUTDATED" <<EOF
{"schema_version": 1,
 "sources": [
  {"kind": "include", "name": "shared", "source": "https://github.com/acme/rules", "constraint": "^1.0",
   "locked": {"tag": "v1.0.0"}, "allowed": {"tag": "v1.2.0"}, "status": "$1"},
  {"kind": "skill", "name": "current", "source": "https://github.com/acme/skills", "constraint": "^2.0",
   "locked": {"tag": "v2.0.0"}, "allowed": {"tag": "v2.0.0"}, "status": "up-to-date"}
 ],
 "summary": {"total": 2, "updatable": 1, "major_available": 0, "held_back": 0, "tag_moved": 0, "errors": 0}}
EOF
}

# A source that was never locked has no locked tag; `update` still pins it.
outdated_not_locked() {
  cat >"$STUB_OUTDATED" <<EOF
{"schema_version": 1,
 "sources": [
  {"kind": "skill", "name": "fresh", "source": "https://github.com/acme/skills", "constraint": "^2.0",
   "allowed": {"tag": "v2.1.0"}, "status": "not-locked"}
 ],
 "summary": {"total": 1, "updatable": 0, "major_available": 0, "held_back": 0, "tag_moved": 0, "errors": 0}}
EOF
}

run_update() {
  cd "$WORK" || return 1
  run bash "$SCRIPT"
}

# Fail when the command succeeds. A bare `! cmd` never fails a Bats test (errexit ignores it).
refute() {
  if "$@"; then
    printf 'expected failure: %s\n' "$*" >&2
    return 1
  fi
}

PR_LIST="gh pr list --head ai-rulez/update --base main --state open --json url --jq .[0].url // \"\""
PR_CREATE="gh pr create --base main --head ai-rulez/update --title chore(ai-rulez): update pinned sources"
UPDATE_STEPS=(
  "ai-rulez lock --outdated --format json"
  "ai-rulez update"
  "ai-rulez generate --no-local"
  "ai-rulez lock --content-only"
)

@test "nothing updatable stops with a notice before update runs" {
  outdated up-to-date

  run_update

  xberg_assert_status 0
  xberg_assert_output "::notice::No ai-rulez source has an update its version constraint allows; nothing to do."
  xberg_assert_trace "ai-rulez lock --outdated --format json"
  xberg_assert_github_output "updated=false"
  [ "$(git -C "$REMOTE" rev-parse main)" = "$(git -C "$WORK" rev-parse HEAD)" ]
  refute git -C "$REMOTE" rev-parse --verify --quiet refs/heads/ai-rulez/update
}

@test "the default pushes the update branch and opens a pull request with the outdated table" {
  base="$(git -C "$WORK" rev-parse HEAD)"

  run_update

  xberg_assert_status 0
  xberg_assert_trace "${UPDATE_STEPS[@]}" \
    "$PR_LIST token=" \
    "$PR_CREATE --body-file BODY token="
  commit="$(git -C "$REMOTE" rev-parse refs/heads/ai-rulez/update)"
  [ "$(git -C "$REMOTE" rev-parse main)" = "$base" ]
  [ "$(git -C "$REMOTE" rev-parse "$commit^")" = "$base" ]
  [ "$(git -C "$REMOTE" show "$commit:.ai-rulez/ai-rulez.lock")" = 'pin = "v1.2.0"' ]
  [ "$(git -C "$REMOTE" log --format='%an <%ae>|%cn' -1 "$commit")" = \
    'github-actions[bot] <41898282+github-actions[bot]@users.noreply.github.com>|github-actions[bot]' ]
  [ "$(git -C "$REMOTE" log --format=%B -1 "$commit")" = "$(printf '%s\n\n%s\n\n%s' \
    'chore(ai-rulez): update pinned sources' 'ai-rulez update moved 1 source(s):' \
    '- include shared (updatable): v1.0.0 -> v1.2.0')" ]
  grep -qxF '| include | shared | https://github.com/acme/rules | updatable | v1.0.0 | v1.2.0 |' "$BATS_TEST_TMPDIR/body.md"
  refute grep -q current "$BATS_TEST_TMPDIR/body.md"
  grep -qxF '| include | shared | https://github.com/acme/rules | updatable | v1.0.0 | v1.2.0 |' "$GITHUB_STEP_SUMMARY"
  xberg_assert_github_output "$(printf 'updated=true\ncommit=%s\npull-request-url=%s' \
    "$commit" https://github.com/owner/repo/pull/7)"
}

@test "a source that was never locked is updated and listed" {
  outdated_not_locked

  run_update

  xberg_assert_status 0
  xberg_assert_trace "${UPDATE_STEPS[@]}" \
    "$PR_LIST token=" \
    "$PR_CREATE --body-file BODY token="
  commit="$(git -C "$REMOTE" rev-parse refs/heads/ai-rulez/update)"
  [ "$(git -C "$REMOTE" log --format=%B -1 "$commit")" = "$(printf '%s\n\n%s\n\n%s' \
    'chore(ai-rulez): update pinned sources' 'ai-rulez update moved 1 source(s):' \
    '- skill fresh (not-locked): - -> v2.1.0')" ]
  grep -qxF '| skill | fresh | https://github.com/acme/skills | not-locked | - | v2.1.0 |' "$BATS_TEST_TMPDIR/body.md"
}

@test "a rerun replaces the update branch and refreshes the open pull request" {
  run_update
  xberg_assert_status 0
  first="$(git -C "$REMOTE" rev-parse refs/heads/ai-rulez/update)"
  git -C "$WORK" checkout -q main
  rm -f "$XBERG_TRACE" "$GITHUB_OUTPUT"
  export STUB_OPEN_PR=https://github.com/owner/repo/pull/7 STUB_PIN=v1.3.0

  run_update

  xberg_assert_status 0
  xberg_assert_trace "${UPDATE_STEPS[@]}" \
    "$PR_LIST token=" \
    "gh pr edit https://github.com/owner/repo/pull/7 --title chore(ai-rulez): update pinned sources --body-file BODY token="
  second="$(git -C "$REMOTE" rev-parse refs/heads/ai-rulez/update)"
  [ "$second" != "$first" ]
  [ "$(git -C "$REMOTE" show "$second:.ai-rulez/ai-rulez.lock")" = 'pin = "v1.3.0"' ]
  [ "$(git -C "$REMOTE" rev-parse "$second^")" = "$(git -C "$REMOTE" rev-parse main)" ]
}

@test "a branch moved by someone else is not overwritten" {
  git -C "$WORK" push -q origin main:refs/heads/ai-rulez/update
  other="$BATS_TEST_TMPDIR/other"
  git clone -q "$REMOTE" "$other"
  git -C "$other" checkout -q ai-rulez/update
  git -C "$other" commit -q --allow-empty -m "someone else"
  # Someone else pushes right after the script reads the branch: a git wrapper does it on ls-remote.
  export REAL_GIT RACE_CLONE="$other"
  REAL_GIT="$(command -v git)"
  xberg_stub git \
    'if [ "$1" = ls-remote ]; then' \
    '  out="$("$REAL_GIT" "$@")" || exit' \
    '  "$REAL_GIT" -C "$RACE_CLONE" push -q origin ai-rulez/update || exit' \
    '  printf "%s\n" "$out"; exit 0' \
    'fi' \
    'exec "$REAL_GIT" "$@"'

  run_update

  xberg_assert_status 1
  xberg_assert_output_contains "(stale info)"
  [ "$("$REAL_GIT" -C "$REMOTE" log --format=%s -1 refs/heads/ai-rulez/update)" = "someone else" ]
}

@test "direct mode pushes to the checked-out branch without a pull request" {
  export INPUT_DIRECT=true
  base="$(git -C "$WORK" rev-parse HEAD)"

  run_update

  xberg_assert_status 0
  xberg_assert_trace "${UPDATE_STEPS[@]}"
  commit="$(git -C "$REMOTE" rev-parse main)"
  [ "$(git -C "$REMOTE" rev-parse "$commit^")" = "$base" ]
  refute git -C "$REMOTE" rev-parse --verify --quiet refs/heads/ai-rulez/update
  xberg_assert_github_output "$(printf 'updated=true\ncommit=%s' "$commit")"
}

@test "names and kind limit both the outdated report and the update" {
  export INPUT_NAMES=$'shared\n  other ' INPUT_KIND=include INPUT_DIRECT=true

  run_update

  xberg_assert_status 0
  xberg_assert_trace \
    "ai-rulez lock --outdated --format json --kind include -- shared other" \
    "ai-rulez update --kind include -- shared other" \
    "ai-rulez generate --no-local" \
    "ai-rulez lock --content-only"
}

@test "the token reaches gh and git only through the environment" {
  export INPUT_TOKEN=secret-token-value

  run_update

  xberg_assert_status 0
  xberg_assert_trace "${UPDATE_STEPS[@]}" \
    "$PR_LIST token=set" \
    "$PR_CREATE --body-file BODY token=set"
  [[ "$output" != *secret-token-value* ]]
  refute grep -q secret-token-value "$XBERG_TRACE" "$GITHUB_OUTPUT" "$WORK/.git/config"
}

@test "an update that changes no file commits nothing" {
  export STUB_NO_CHANGE=1

  run_update

  xberg_assert_status 0
  xberg_assert_output "::notice::ai-rulez update changed no file; nothing to commit."
  xberg_assert_github_output "updated=false"
  refute git -C "$REMOTE" rev-parse --verify --quiet refs/heads/ai-rulez/update
}

@test "a refused update fails the job and pushes nothing" {
  export STUB_UPDATE_EXIT=2

  run_update

  xberg_assert_status 2
  xberg_assert_trace "ai-rulez lock --outdated --format json" "ai-rulez update"
  refute git -C "$REMOTE" rev-parse --verify --quiet refs/heads/ai-rulez/update
}

@test "uncommitted changes in the checkout are refused" {
  printf 'stray\n' >"$WORK/stray.txt"

  run_update

  xberg_assert_status 1
  xberg_assert_output "::error::the checkout has uncommitted changes; the update commits everything it changes"
  xberg_assert_trace_empty
}

@test "invalid inputs are rejected before any command runs" {
  local cases=(
    "INPUT_DIRECT=yes|direct must be true or false"
    "INPUT_KIND=served|kind must be include, skill, source, marketplace or plugin"
    "INPUT_NAMES=--accept-findings|names must be source names (letters, digits, . _ @ / -)"
    "INPUT_NAMES=*|names must be source names (letters, digits, . _ @ / -)"
    "INPUT_BRANCH=bad..name|branch must be a valid branch name"
    "INPUT_BRANCH=-x|branch must be a valid branch name"
    "INPUT_BRANCH=main|branch must differ from the checked-out branch main"
    "INPUT_WORKING_DIRECTORY=/tmp|working-directory must be a relative path"
    "INPUT_WORKING_DIRECTORY=../x|working-directory must not contain .."
  )
  local case assignment message
  for case in "${cases[@]}"; do
    assignment="${case%%|*}"
    message="${case#*|}"
    cd "$WORK" || return 1
    run env "$assignment" bash "$SCRIPT"
    xberg_assert_status 1
    xberg_assert_output "::error::$message"
  done
  xberg_assert_trace_empty
}

@test "a detached HEAD is refused" {
  git -C "$WORK" checkout -q --detach

  run_update

  xberg_assert_status 1
  xberg_assert_output "::error::HEAD is detached; check out the branch to update"
}
