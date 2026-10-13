#!/usr/bin/env bash
# Builds the monorepo the monorepo tape records in. Usage: monorepo.fixture.sh <dir>
# The directory is created fresh; nothing here touches the network or the clock.
set -euo pipefail

dir="${1:?usage: monorepo.fixture.sh <dir>}"

rm -rf "$dir"
mkdir -p "$dir"
cd "$dir"
git init -q
git config user.email demo@example.com
git config user.name demo

# write_rule <config-dir> <id> <body>
write_rule() {
	local cdir="$1" id="$2" body="$3"
	mkdir -p "$cdir/rules"
	{
		echo "---"
		echo "priority: high"
		echo "---"
		echo
		printf '%s\n' "$body"
	} >"$cdir/rules/$id.md"
}

# Root config plus one nested root per service; each root generates for its own tools.
mkdir -p .ai-rulez services/api/.ai-rulez web/.ai-rulez
cat >.ai-rulez/config.toml <<'TOML'
version = "5.0"
name = "acme"
presets = ["claude", "codex"]
TOML
cat >services/api/.ai-rulez/config.toml <<'TOML'
version = "5.0"
name = "api"
presets = ["claude", "codex"]
TOML
cat >web/.ai-rulez/config.toml <<'TOML'
version = "5.0"
name = "web"
presets = ["claude"]
TOML
write_rule .ai-rulez security "Never commit secrets. Validate all input."
write_rule services/api/.ai-rulez api-style "Handlers return typed errors. No raw SQL."
write_rule web/.ai-rulez web-style "Components are function components with typed props."

# A long always-on rule, held back as a draft for the tape to adopt later.
{
	printf -- '---\npriority: high\n---\n\n# Style Guide\n\n'
	for _ in $(seq 1 40); do
		echo "Follow the shared style guide in every module and keep naming consistent across packages."
	done
} >style-guide.draft

git add -A
git commit -q -m "fixture"
