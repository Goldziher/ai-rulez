#!/usr/bin/env bats
# The Husky pre-commit hook `ai-rulez init --setup-hooks` writes (internal/hooks/setup.go).
#
# Each test runs init through the binary in a scratch project (HOME and XDG redirected), then
# executes .husky/pre-commit as git would, with npx stubbed so no package is fetched.

setup() {
  bats_load_library xberg-bats
  load "${BATS_TEST_DIRNAME}/../helpers/common"
  xberg_setup
  isolate_home
  PROJECT="$XBERG_WORK/husky project"
  mkdir -p "$PROJECT/.husky"
  git -C "$PROJECT" init -q
  HOOK="$PROJECT/.husky/pre-commit"
  xberg_stub_trace npx
}

setup_hooks() {
  (cd "$PROJECT" && "$(ai_rulez_bin)" init --setup-hooks --yes) >"$BATS_TEST_TMPDIR/init.log" 2>&1 || {
    cat "$BATS_TEST_TMPDIR/init.log" >&2
    return 1
  }
}

# Husky v4-v8 ship .husky/_/husky.sh, which the hook sources.
husky_v8_layout() {
  mkdir -p "$PROJECT/.husky/_"
  printf '%s\n' '#!/usr/bin/env sh' >"$PROJECT/.husky/_/husky.sh"
}

@test "npx resolves to the stub before any host copy" {
  [ "$(command -v npx)" = "$XBERG_STUB_BIN/npx" ]
}

@test "a fresh hook is executable and validates through npx" {
  husky_v8_layout
  setup_hooks

  [ -x "$HOOK" ]
  run "$HOOK"

  xberg_assert_status 0
  xberg_assert_output "Validating AI rules..."
  xberg_assert_trace "npx ai-rulez validate"
}

@test "a failing validation fails the hook, so the commit is blocked" {
  husky_v8_layout
  setup_hooks
  xberg_stub_exit npx 2

  run "$HOOK"

  xberg_assert_status 1
}

@test "an existing hook keeps its commands and runs validation after them" {
  printf 'npm test' >"$HOOK"
  chmod +x "$HOOK"
  setup_hooks
  xberg_stub_trace npm

  run sh -e "$HOOK"

  xberg_assert_status 0
  xberg_assert_trace "npm test" "npx ai-rulez validate"
}

@test "a hook that already mentions ai-rulez is left untouched" {
  printf '%s\n' 'npx ai-rulez generate --check' >"$HOOK"
  setup_hooks

  xberg_assert_file "$HOOK" "npx ai-rulez generate --check"
}

@test "a fresh hook runs on a Husky v9+ layout, which no longer ships husky.sh" {
  known_bug "internal/hooks/setup.go sources .husky/_/husky.sh, deprecated in Husky v9 and removed in v10; drop the two-line header"
  mkdir -p "$PROJECT/.husky/_"
  printf '%s\n' '#!/usr/bin/env sh' >"$PROJECT/.husky/_/h"
  setup_hooks

  run sh -e "$HOOK"

  xberg_assert_status 0
  xberg_assert_trace "npx ai-rulez validate"
}
