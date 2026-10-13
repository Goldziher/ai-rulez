#!/usr/bin/env bash
# Builds the throwaway project the plugin-drift tape records in. Usage: plugin-drift.fixture.sh <dir>
# The directory is created fresh; nothing here touches the network or the clock.
set -euo pipefail

dir="${1:?usage: plugin-drift.fixture.sh <dir>}"

rm -rf "$dir"
mkdir -p "$dir"
cd "$dir"
git init -q
git config user.email demo@example.com
git config user.name demo

ai-rulez init acme --yes >/dev/null 2>&1
rm -rf .ai-rulez/skills/ai-rulez .ai-rulez/skills/code-reviewer

cat >.ai-rulez/config.toml <<'TOML'
version = "5.0"
name = "acme"
presets = ["claude"]

[plugin]
name = "acme"
version = "1.0.0"
description = "Acme team skills."
runtimes = ["claude"]
skills_dir_owned = true
TOML

skill() {
  local id="$1" title="$2" desc="$3" body="$4"
  mkdir -p ".ai-rulez/skills/$id"
  cat >".ai-rulez/skills/$id/SKILL.md" <<MD
---
type: Playbook
title: $title
description: $desc
x-ai-rulez:
  kind: skill
  id: $id
---

$body
MD
  printf '# Concepts\n\n* [%s](SKILL.md) - %s\n' "$title" "$desc" >".ai-rulez/skills/$id/index.md"
}

skill code-review "Code Review" "Use when reviewing a pull request for correctness and style." "Read the diff first, then check tests and naming."
skill release-notes "Release Notes" "Use when drafting release notes from merged pull requests." "Group changes by user impact and link each pull request."
printf '# Subdirectories\n\n* [code-review](code-review/index.md)\n* [release-notes](release-notes/index.md)\n' >.ai-rulez/skills/index.md
