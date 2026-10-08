#!/usr/bin/env bash
# Repro: `validate --config-only` has no offline mode. A project with a remote
# git include that cannot be reached fails to validate, while
# `generate --offline` degrades (warns, skips the include) and succeeds.
#
# usage: validate-remote-include-offline.sh /path/to/ai-rulez
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

[[includes]]
name = "remote"
source = "https://invalid.invalid/org/conventions.git"
TOML
cd "$T/p"
"$BIN" generate --offline >/dev/null 2>&1 || {
  echo "generate --offline failed too; repro does not apply"
  exit 0
}
if "$BIN" validate --config-only >/dev/null 2>&1; then
  echo "OK: validate --config-only works with an unreachable remote include"
  exit 0
fi
echo "REPRODUCED: generate --offline succeeds, validate --config-only has no offline mode and fails"
exit 1
