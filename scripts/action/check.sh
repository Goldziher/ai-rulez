#!/usr/bin/env bash

# The governance gate behind the check sub-action (check/action.yml). For every ai-rulez project
# root, in order: migrate okf --check, validate, lock --check (when a lock exists), generate, then
# generate --check, optionally verify --plugin and tokens --budget. Afterwards the checkout must
# match HEAD with no new untracked files, and at least one tracked file must have been examined so
# an empty checkout cannot pass. Writes a job summary and roots= / files-examined= outputs.
#
# Inputs (environment, set by the action):
#   INPUT_CONFIG_DIRECTORIES   newline list of configuration directories (.ai-rulez, .agents,
#                              .config/ai-rulez, or the project root holding one); empty discovers
#                              every tracked configuration
#   INPUT_EXPECTED_CONFIG_COUNT  positive integer; fail when the number of roots differs
#   INPUT_PLUGIN, INPUT_STRICT, INPUT_LOCAL  true or false
#   INPUT_TOKEN_BUDGET         positive integer passed to tokens --budget; empty skips it
set -euo pipefail

die() {
  printf '::error::%s\n' "$1" >&2
  exit 1
}

bool_input() {
  case "$2" in
  true | false) printf '%s' "$2" ;;
  *) die "$1 must be true or false" ;;
  esac
}

int_input() {
  [[ -z "$2" || "$2" =~ ^[1-9][0-9]{0,8}$ ]] || die "$1 must be a positive integer"
  printf '%s' "$2"
}

plugin="$(bool_input plugin "${INPUT_PLUGIN:-false}")"
strict="$(bool_input strict "${INPUT_STRICT:-true}")"
local_overlay="$(bool_input local "${INPUT_LOCAL:-false}")"
expected="$(int_input expected-config-count "${INPUT_EXPECTED_CONFIG_COUNT:-}")"
budget="$(int_input token-budget "${INPUT_TOKEN_BUDGET:-}")"
summary_file="${GITHUB_STEP_SUMMARY:-/dev/null}"
output_file="${GITHUB_OUTPUT:-/dev/null}"

git rev-parse --is-inside-work-tree >/dev/null 2>&1 || die "the check needs a git checkout"

has_config() {
  [[ -f "$1/.agents/ai-rulez.toml" || -f "$1/.ai-rulez/config.toml" || -f "$1/.config/ai-rulez/config.toml" ]]
}

# The project root a configuration directory belongs to; a project root maps to itself.
root_of() {
  local entry="$1"
  case "$entry" in
  .ai-rulez | .agents | .config/ai-rulez) printf '.' ;;
  */.ai-rulez) printf '%s' "${entry%/.ai-rulez}" ;;
  */.agents) printf '%s' "${entry%/.agents}" ;;
  */.config/ai-rulez) printf '%s' "${entry%/.config/ai-rulez}" ;;
  *) printf '%s' "$entry" ;;
  esac
}

config_directories="${INPUT_CONFIG_DIRECTORIES:-}"
candidates=()
if [[ -n "${config_directories//[[:space:]]/}" ]]; then
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    [[ -n "$line" ]] || continue
    case "$line" in /* | '~'*) die "config-directories entries must be relative paths" ;; esac
    case "/$line/" in */../*) die "config-directories entries must not contain .." ;; esac
    while [[ "$line" == */ && "$line" != / ]]; do line="${line%/}"; done
    while [[ "$line" == ./* ]]; do line="${line#./}"; done
    root="$(root_of "$line")"
    [[ -n "$root" ]] || root="."
    [[ -d "$root" ]] || die "config-directories: $line is not a directory"
    has_config "$root" || die "config-directories: no ai-rulez configuration in $root"
    candidates+=("$root")
  done <<<"$config_directories"
else
  while IFS= read -r -d '' file; do
    case "$file" in
    */.agents/ai-rulez.toml) root="${file%/.agents/ai-rulez.toml}" ;;
    .agents/ai-rulez.toml | .ai-rulez/config.toml | .config/ai-rulez/config.toml) root="." ;;
    */.config/ai-rulez/config.toml) root="${file%/.config/ai-rulez/config.toml}" ;;
    */.ai-rulez/config.toml) root="${file%/.ai-rulez/config.toml}" ;;
    *) continue ;;
    esac
    # .agents/.ai-rulez holds the agents source's lock and manifest, not another project.
    case "$root" in .agents | */.agents) continue ;; esac
    candidates+=("$root")
  done < <(git ls-files -z -- ':(glob)**/.ai-rulez/config.toml' ':(glob)**/.config/ai-rulez/config.toml' \
    ':(glob)**/.agents/ai-rulez.toml')
fi

roots=()
while IFS= read -r root; do
  if [[ -n "$root" ]]; then roots+=("$root"); fi
done < <(printf '%s\n' ${candidates[@]+"${candidates[@]}"} | LC_ALL=C sort -u)

count="${#roots[@]}"
[[ "$count" -gt 0 ]] || die "no ai-rulez configuration found; set config-directories or commit one"
if [[ -n "$expected" && "$count" -ne "$expected" ]]; then
  die "expected $expected configuration directories but found $count: ${roots[*]}"
fi

rows=()
result="failed"
failure=""
files_examined=0

write_summary() {
  {
    printf '## ai-rulez check\n\n'
    if [[ "$result" == passed ]]; then
      printf 'Result: **passed**\n\n'
    else
      printf 'Result: **failed**%s\n\n' "${failure:+ at \`$failure\`}"
    fi
    printf -- '- Roots checked: %s%s\n' "$count" "${expected:+ (expected $expected)}"
    printf -- '- Tracked files examined: %s\n\n' "$files_examined"
    printf '| Root | Command | Result |\n| --- | --- | --- |\n'
    local row
    for row in ${rows[@]+"${rows[@]}"}; do printf '%s\n' "$row"; done
  } >>"$summary_file"
}
trap write_summary EXIT

# Run one ai-rulez command in a root, recording it for the summary; the first failure stops the gate.
run_in() {
  local root="$1" status=0
  shift
  local display="ai-rulez $*"
  echo "::group::[$root] $display"
  (cd "$root" && ai-rulez "$@") || status=$?
  echo "::endgroup::"
  if [[ "$status" -ne 0 ]]; then
    rows+=("| \`$root\` | \`$display\` | failed (exit $status) |")
    failure="[$root] $display"
    die "[$root] $display failed with exit $status"
  fi
  rows+=("| \`$root\` | \`$display\` | passed |")
}

skip_in() {
  rows+=("| \`$1\` | \`$2\` | skipped: $3 |")
}

agents_source() {
  local root="$1" config
  [[ -f "$root/.agents/ai-rulez.toml" ]] && return 0
  for config in "$root/.ai-rulez/config.toml" "$root/.config/ai-rulez/config.toml"; do
    [[ -f "$config" ]] && grep -Eq '^[[:space:]]*source[[:space:]]*=[[:space:]]*"agents"' "$config" && return 0
  done
  return 1
}

has_lock() {
  [[ -f "$1/.ai-rulez/ai-rulez.lock" || -f "$1/.config/ai-rulez/ai-rulez.lock" ||
    -f "$1/.agents/.ai-rulez/ai-rulez.lock" ]]
}

validate_flags=()
generate_flags=()
if [[ "$strict" == true ]]; then
  validate_flags+=(--strict)
  generate_flags+=(--strict-config)
fi
if [[ "$local_overlay" == false ]]; then
  validate_flags+=(--no-local)
  generate_flags+=(--no-local)
fi

# The clean-checkout check compares against HEAD and examines every tracked file.
git rev-parse --verify --quiet HEAD >/dev/null || die "the checkout has no commit to compare against"
files_examined="$(git ls-files | wc -l | tr -d '[:space:]')"
if [[ "$files_examined" -eq 0 ]]; then
  failure="git ls-files"
  die "no tracked files to examine; an empty checkout cannot pass"
fi
untracked_before="$(git ls-files --others --exclude-standard)"

for root in "${roots[@]}"; do
  if agents_source "$root"; then
    skip_in "$root" "ai-rulez migrate okf --check" 'source = "agents"'
  else
    run_in "$root" migrate okf --check
  fi
  run_in "$root" validate ${validate_flags[@]+"${validate_flags[@]}"}
  if has_lock "$root"; then
    run_in "$root" lock --check
  else
    skip_in "$root" "ai-rulez lock --check" "no ai-rulez.lock"
  fi
  run_in "$root" generate ${generate_flags[@]+"${generate_flags[@]}"}
  run_in "$root" generate --check ${generate_flags[@]+"${generate_flags[@]}"}
  if [[ "$plugin" == true ]]; then run_in "$root" verify --plugin; fi
  if [[ -n "$budget" ]]; then
    tokens_flags=(--budget "$budget")
    if [[ "$local_overlay" == false ]]; then tokens_flags+=(--no-local); fi
    run_in "$root" tokens "${tokens_flags[@]}"
  fi
done

echo "::group::git diff --exit-code HEAD"
diff_status=0
git diff --exit-code --stat HEAD || diff_status=$?
echo "::endgroup::"
if [[ "$diff_status" -ne 0 ]]; then
  rows+=("| \`.\` | \`git diff --exit-code HEAD\` | failed |")
  failure="git diff --exit-code HEAD"
  die "generation changed tracked files; run the commands above locally and commit the result"
fi
rows+=("| \`.\` | \`git diff --exit-code HEAD\` | passed |")

new_untracked="$(comm -13 <(printf '%s\n' "$untracked_before" | LC_ALL=C sort) \
  <(git ls-files --others --exclude-standard | LC_ALL=C sort) | sed '/^$/d')"
if [[ -n "$new_untracked" ]]; then
  rows+=("| \`.\` | no new untracked files | failed |")
  failure="untracked files"
  printf '%s\n' "$new_untracked" >&2
  die "generation left untracked files; commit them or ignore them"
fi
rows+=("| \`.\` | no new untracked files | passed |")

result="passed"
{
  echo "roots=$count"
  echo "files-examined=$files_examined"
} >>"$output_file"
echo "ai-rulez check passed: $count root(s), $files_examined tracked file(s) examined"
