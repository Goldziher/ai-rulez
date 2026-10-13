#!/usr/bin/env bash
# Builds the throwaway project a README tape records in. Usage: fixture.sh <name> <dir>
# The directory is created fresh; nothing here touches the network or the clock.
set -euo pipefail

name="${1:?usage: fixture.sh <name> <dir>}"
dir="${2:?usage: fixture.sh <name> <dir>}"

rm -rf "$dir"
mkdir -p "$dir"
cd "$dir"
git init -q
git config user.email demo@example.com
git config user.name demo

case "$name" in
quickstart)
	ai-rulez init acme --yes >/dev/null 2>&1
	python3 - <<'PY'
import pathlib

p = pathlib.Path(".ai-rulez/config.toml")
p.write_text(p.read_text().replace('presets = ["claude"]', 'presets = ["claude", "cursor", "codex", "zed"]', 1))
PY
	;;
*)
	echo "unknown fixture: $name" >&2
	exit 2
	;;
esac
