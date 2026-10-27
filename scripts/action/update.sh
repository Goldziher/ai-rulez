#!/usr/bin/env bash

# The scheduled update behind the update sub-action (update/action.yml): move version-range pins
# with `ai-rulez update`, regenerate, re-pin the content, then commit the result to a branch and
# open (or refresh) a pull request, or push it straight to the checked-out branch.
#
#   1. ai-rulez lock --outdated --format json; stop with a notice when nothing is updatable
#   2. ai-rulez update (the pre-pin security scan applies; a refusal fails with its AR code)
#   3. ai-rulez generate --no-local, then ai-rulez lock --content-only
#   4. commit as github-actions[bot]; push to INPUT_BRANCH and open a PR, or push to the base
#
# Inputs (environment, set by the action):
#   INPUT_NAMES              sources to update, separated by spaces or newlines; empty means all
#   INPUT_KIND               include, skill, source, marketplace or plugin; empty means all
#   INPUT_DIRECT             true pushes to the checked-out branch; false (default) opens a PR
#   INPUT_BRANCH             the pull request branch (default ai-rulez/update)
#   INPUT_TOKEN              token for git push and gh; empty keeps the checkout's credentials
#   INPUT_WORKING_DIRECTORY  the project root, relative to the workspace (default .)
# The token reaches git through GIT_CONFIG_* variables and gh through GH_TOKEN, never argv.
set -euo pipefail

die() {
  printf '::error::%s\n' "$1" >&2
  exit 1
}

names_input="${INPUT_NAMES:-}"
kind="${INPUT_KIND:-}"
direct="${INPUT_DIRECT:-false}"
branch="${INPUT_BRANCH:-ai-rulez/update}"
token="${INPUT_TOKEN:-}"
workdir="${INPUT_WORKING_DIRECTORY:-.}"
summary_file="${GITHUB_STEP_SUMMARY:-/dev/null}"
output_file="${GITHUB_OUTPUT:-/dev/null}"

case "$direct" in true | false) ;; *) die "direct must be true or false" ;; esac
case "$kind" in "" | include | skill | source | marketplace | plugin) ;; *)
  die "kind must be include, skill, source, marketplace or plugin"
  ;;
esac
names=()
set -f # split the list on whitespace without expanding a glob
for name in $names_input; do
  [[ "$name" =~ ^[A-Za-z0-9_][A-Za-z0-9._@/-]*$ ]] || die "names must be source names (letters, digits, . _ @ / -)"
  names+=("$name")
done
set +f
if ! [[ "$branch" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ ]] || ! git check-ref-format --branch "$branch" >/dev/null 2>&1; then
  die "branch must be a valid branch name"
fi
case "$workdir" in /* | '~'*) die "working-directory must be a relative path" ;; esac
case "/$workdir/" in */../*) die "working-directory must not contain .." ;; esac
[[ -d "$workdir" ]] || die "working-directory is not a directory"
cd "$workdir"

git rev-parse --is-inside-work-tree >/dev/null 2>&1 || die "the update needs a git checkout"
base="$(git symbolic-ref --quiet --short HEAD)" || die "HEAD is detached; check out the branch to update"
[[ "$direct" == true || "$branch" != "$base" ]] || die "branch must differ from the checked-out branch $base"
[[ -z "$(git status --porcelain)" ]] || die "the checkout has uncommitted changes; the update commits everything it changes"

filters=()
if [[ -n "$kind" ]]; then filters+=(--kind "$kind"); fi
if [[ "${#names[@]}" -gt 0 ]]; then filters+=(-- "${names[@]}"); fi

# lock --outdated and update read release dates for min_release_age with GITHUB_TOKEN (github.com only).
if [[ -n "$token" && -z "${GITHUB_TOKEN:-}" ]]; then export GITHUB_TOKEN="$token"; fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

ai-rulez lock --outdated --format json ${filters[@]+"${filters[@]}"} >"$work/outdated.json"
# The statuses `ai-rulez update` moves (pinupdate.Plan): an allowed update, a source never locked, a locked tag gone.
movable='def movable: select(.status == "updatable" or .status == "not-locked" or .status == "tag-missing");'
updatable="$(jq "$movable [.sources[] | movable] | length" "$work/outdated.json")"
if [[ "$updatable" -eq 0 ]]; then
  echo "::notice::No ai-rulez source has an update its version constraint allows; nothing to do."
  printf '## ai-rulez update\n\nNo source has an allowed update.\n' >>"$summary_file"
  echo "updated=false" >>"$output_file"
  exit 0
fi

# The table for the PR body and the job summary, and a plain list for the commit message.
jq -r "$movable"'
  def cell: tostring | gsub("[|]"; "\\|") | gsub("[\r\n]"; " ");
  "| Kind | Name | Source | Status | Locked | New |",
  "| --- | --- | --- | --- | --- | --- |",
  (.sources[] | movable
    | "| \(.kind | cell) | \(.name | cell) | \(.source | cell) | \(.status | cell) | \(.locked.tag // "-" | cell) | \(.allowed.tag // "-" | cell) |")
' "$work/outdated.json" >"$work/table.md"
jq -r "$movable"'.sources[] | movable
  | "- \(.kind) \(.name) (\(.status)): \(.locked.tag // "-") -> \(.allowed.tag // "-")"' "$work/outdated.json" >"$work/list.txt"

ai-rulez update ${filters[@]+"${filters[@]}"}
ai-rulez generate --no-local
ai-rulez lock --content-only

if [[ -z "$(git status --porcelain)" ]]; then
  echo "::notice::ai-rulez update changed no file; nothing to commit."
  printf '## ai-rulez update\n\nThe update changed no file.\n' >>"$summary_file"
  echo "updated=false" >>"$output_file"
  exit 0
fi

export GIT_AUTHOR_NAME="github-actions[bot]" GIT_COMMITTER_NAME="github-actions[bot]"
export GIT_AUTHOR_EMAIL="41898282+github-actions[bot]@users.noreply.github.com"
export GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"
if [[ -n "$token" ]]; then
  # Reset the header a checkout persisted, then send this token's; neither reaches argv or a file.
  basic="$(printf 'x-access-token:%s' "$token" | base64 | tr -d '\n')"
  if [[ "${GITHUB_ACTIONS:-}" == true ]]; then echo "::add-mask::$basic"; fi
  header_key="http.${GITHUB_SERVER_URL:-https://github.com}/.extraheader"
  export GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0="$header_key" GIT_CONFIG_VALUE_0=""
  export GIT_CONFIG_KEY_1="$header_key" GIT_CONFIG_VALUE_1="AUTHORIZATION: basic $basic"
  export GH_TOKEN="$token"
fi
if [[ -n "${GITHUB_REPOSITORY:-}" ]]; then export GH_REPO="$GITHUB_REPOSITORY"; fi

title="chore(ai-rulez): update pinned sources"
{
  printf '%s\n\n' "$title"
  printf 'ai-rulez update moved %s source(s):\n\n' "$updatable"
  cat "$work/list.txt"
} >"$work/message.txt"
if [[ "$direct" == false ]]; then git checkout --quiet -B "$branch"; fi
git add -A -- .
git commit --quiet --file "$work/message.txt"
commit="$(git rev-parse HEAD)"

{
  printf '## ai-rulez update\n\n'
  cat "$work/table.md"
  printf '\n'
} >>"$summary_file"

if [[ "$direct" == true ]]; then
  git push --quiet origin "HEAD:refs/heads/$base"
  {
    echo "updated=true"
    echo "commit=$commit"
  } >>"$output_file"
  echo "Pushed $commit to $base"
  exit 0
fi

# The branch belongs to this action: replace it, but only from the value read here, so a push by
# someone else in between fails instead of being lost.
previous="$(git ls-remote origin "refs/heads/$branch" | cut -f1)"
git push --quiet --force-with-lease="refs/heads/$branch:$previous" origin "HEAD:refs/heads/$branch"

{
  # shellcheck disable=SC2016 # the backticks are Markdown, not a command substitution
  printf '`ai-rulez update` found %s source(s) with an update their version constraint allows.\n\n' "$updatable"
  cat "$work/table.md"
  printf '\nThe lock, the regenerated outputs and the content pins are in this commit. '
  printf 'Review the source changes before merging.\n'
} >"$work/body.md"

url="$(gh pr list --head "$branch" --base "$base" --state open --json url --jq '.[0].url // ""')"
if [[ -n "$url" ]]; then
  gh pr edit "$url" --title "$title" --body-file "$work/body.md" >/dev/null
else
  url="$(gh pr create --base "$base" --head "$branch" --title "$title" --body-file "$work/body.md")"
fi
{
  echo "updated=true"
  echo "commit=$commit"
  echo "pull-request-url=$url"
} >>"$output_file"
echo "Pull request: $url"
