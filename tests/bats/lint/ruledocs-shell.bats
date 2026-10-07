#!/usr/bin/env bats
# The shell remedy the rule docs recommend for AR005 risky-shell-exec (internal/lint/ruledocs.go),
# shown by `validate --explain`, in SARIF rule help and on the docs site.
#
# The commands are read back through the binary, then checked two ways: the remedy must not trip
# the rule it is the remedy for, and run with a stubbed download it must refuse a payload whose
# checksum does not match.

bats_require_minimum_version 1.5.0

setup() {
  bats_load_library xberg-bats
  load "${BATS_TEST_DIRNAME}/../helpers/common"
  xberg_setup
  isolate_home
  command -v jq >/dev/null || fail "jq is required to read the --explain JSON"

  local doc
  doc="$("$(ai_rulez_bin)" validate --explain AR005 --format json)"
  # "Download, pin a checksum, inspect, then run: `cmd`" -> cmd
  GOOD="$(printf '%s' "$doc" | jq -r .good | sed -n 's/^[^`]*`\(.*\)`$/\1/p')"
  BAD="$(printf '%s' "$doc" | jq -r .bad | sed -n 's/^`\(.*\)`$/\1/p')"
  [ -n "$GOOD" ] && [ -n "$BAD" ]

  # A curl that writes $PAYLOAD to its -o target, including a combined flag such as -fsSLo.
  export PAYLOAD='touch ran'
  xberg_stub curl \
    'target="" next=0' \
    'for a in "$@"; do' \
    '  [ "$next" = 1 ] && { target="$a"; next=0; continue; }' \
    '  case "$a" in --output | -o | -[a-zA-Z]*o) next=1 ;; esac' \
    'done' \
    '[ -n "$target" ] || exit 2' \
    'printf "%s\n" "$PAYLOAD" >"$target"'
}

# A scratch project holding one skill whose SKILL.md and script both carry $1.
scan_snippet() {
  local project="$XBERG_WORK/scan project" skill
  skill="$project/.ai-rulez/skills/install"
  make_project "$project" '"claude"'
  mkdir -p "$skill/scripts"
  printf '%s\n' '---' 'name: install' 'description: Use when installing the tool from its release' '---' '' \
    '# Install' '' '```sh' "$1" '```' >"$skill/SKILL.md"
  printf '%s\n' '#!/bin/sh' "$1" >"$skill/scripts/install.sh"
  chmod +x "$skill/scripts/install.sh"
  cd "$project" || return
  run --separate-stderr "$(ai_rulez_bin)" scan --format json
}

need_sha256sum() {
  command -v sha256sum >/dev/null || skip "the remedy calls sha256sum, which this host does not ship"
}

@test "the AR005 remedy is not itself flagged by AR005" {
  scan_snippet "$GOOD"

  xberg_assert_status 0
  [ "$(printf '%s' "$output" | jq '[.findings[] | select(.code == "AR005")] | length')" = 0 ]
}

@test "the AR005 bad example is flagged in both the skill and its script" {
  scan_snippet "$BAD"

  xberg_assert_status 2
  [ "$(printf '%s' "$output" | jq -c '[.findings[] | select(.code == "AR005") | .file] | sort')" = \
    '[".ai-rulez/skills/install/SKILL.md",".ai-rulez/skills/install/scripts/install.sh"]' ]
}

@test "the remedy runs the download when its pinned checksum matches" {
  need_sha256sum
  cd "$XBERG_WORK"
  printf '%s\n' "$PAYLOAD" >expected.sh
  printf '%s  install.sh\n' "$(sha256sum expected.sh | awk '{print $1}')" >install.sha256

  run sh -c "$GOOD"

  xberg_assert_status 0
  [ -f ran ]
}

@test "the remedy refuses a download whose checksum does not match" {
  need_sha256sum
  cd "$XBERG_WORK"
  printf '%s  install.sh\n' "$(printf 'something else\n' | sha256sum | awk '{print $1}')" >install.sha256

  run sh -c "$GOOD"

  [ "$status" -ne 0 ]
  xberg_assert_file_absent ran
}
