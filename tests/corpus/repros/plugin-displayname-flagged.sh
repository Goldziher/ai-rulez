#!/usr/bin/env bash
# Repro: `generate --plugin` writes "displayName" into .claude-plugin/plugin.json
# when [plugin] sets display_name, and `validate` then flags that same field as
# AR963 "unknown field ... is ignored at load time".
#
# usage: plugin-displayname-flagged.sh /path/to/ai-rulez
# exit 0: bug absent; exit 1: bug reproduced.
set -eu
BIN="${1:?usage: $0 /path/to/ai-rulez}"
case "$BIN" in /*) ;; *) BIN="$PWD/$BIN" ;; esac
T="$(mktemp -d "${TMPDIR:-/tmp}/corpus-repro-XXXXXX")"
trap 'rm -rf "${T:?}"' EXIT
export HOME="$T/home" GIT_CONFIG_GLOBAL="$T/gitconfig" GIT_CONFIG_NOSYSTEM=1
mkdir -p "$HOME" "$T/p/.ai-rulez/rules"
: >"$GIT_CONFIG_GLOBAL"
printf '# Rule\n\nbody\n' >"$T/p/.ai-rulez/rules/r.md"
cat >"$T/p/.ai-rulez/config.toml" <<'TOML'
version = "5.0"
name = "repro"
presets = ["claude"]

[plugin]
name = "repro"
version = "1.0.0"
display_name = "Repro Plugin"
description = "repro"
TOML
cd "$T/p"
"$BIN" generate --offline >/dev/null 2>&1
"$BIN" generate --plugin --offline >/dev/null 2>&1
grep -q displayName .claude-plugin/plugin.json || {
  echo "OK: no displayName emitted"
  exit 0
}
if "$BIN" validate 2>&1 | grep -q 'AR963.*displayName'; then
  echo "REPRODUCED: generated displayName is flagged AR963 by validate"
  exit 1
fi
echo "OK: displayName not flagged"
