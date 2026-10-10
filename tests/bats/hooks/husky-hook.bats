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
  # A self-contained stub, not xberg_stub_trace: the hook resolves npx from PATH, and a stub
  # whose shebang re-resolves its interpreter can fall through to the host's node-based npx
  # (the exit-139 crash). See ai_rulez_write_stub in helpers/common.bash.
  ai_rulez_stub_trace npx
  ai_rulez_stub_first npx
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

  # The stub must start without a PATH lookup. An `env` shebang re-resolves its interpreter and
  # is exactly how a stubbed npx once became the host's node-based npx (exit 139).
  run head -n 1 "$XBERG_STUB_BIN/npx"
  xberg_assert_status 0
  xberg_assert_output "#!/bin/sh"
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
  ai_rulez_stub_exit npx 2

  run "$HOOK"

  xberg_assert_status 1
}

@test "an existing hook keeps its commands and runs validation after them" {
  printf 'npm test' >"$HOOK"
  chmod +x "$HOOK"
  setup_hooks
  ai_rulez_stub_trace npm
  ai_rulez_stub_first npm

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
  mkdir -p "$PROJECT/.husky/_"
  printf '%s\n' '#!/usr/bin/env sh' >"$PROJECT/.husky/_/h"
  setup_hooks

  run sh -e "$HOOK"

  xberg_assert_status 0
  xberg_assert_trace "npx ai-rulez validate"
}
