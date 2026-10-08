#!/usr/bin/env bash
# Repro: a file:// git include that names a repository inside the project is
# rejected as "outside the project" when the project is reached through a
# symlinked directory (macOS $TMPDIR is /var/... -> /private/var/...).
#
# usage: symlinked-project-file-url.sh /path/to/ai-rulez
# exit 0: bug absent; exit 1: bug reproduced.
set -eu
BIN="${1:?usage: $0 /path/to/ai-rulez}"
case "$BIN" in /*) ;; *) BIN="$PWD/$BIN" ;; esac
T="$(mktemp -d "${TMPDIR:-/tmp}/corpus-repro-XXXXXX")"
trap 'rm -rf "${T:?}"' EXIT
mkdir -p "$T/real"
ln -s "$T/real" "$T/link"
export HOME="$T/home" GIT_CONFIG_GLOBAL="$T/gitconfig" GIT_CONFIG_NOSYSTEM=1
mkdir -p "$HOME"
printf '[user]\n\tname = repro\n\temail = repro@example.invalid\n[protocol "file"]\n\tallow = always\n' >"$GIT_CONFIG_GLOBAL"

proj="$T/link/proj" # logical path through the symlink
mkdir -p "$proj/conv/rules" "$proj/.ai-rulez"
printf '# Rule\n\nbody\n' >"$proj/conv/rules/r.md"
git -C "$proj/conv" init -q .
git -C "$proj/conv" add -A
git -C "$proj/conv" commit -q -m one
cat >"$proj/.ai-rulez/config.toml" <<TOML
version = "5.0"
name = "repro"
presets = ["claude"]

[[includes]]
name = "conv"
source = "file://$proj/conv"
TOML
cd "$proj"
# Without PWD the binary sees the physical working directory while the config
# names the project through the symlink, which is how a build tool or a
# subshell with a scrubbed environment reaches it.
if out="$(env -u PWD "$BIN" generate --offline 2>&1)"; then
  echo "OK: include accepted"
  exit 0
fi
printf '%s\n' "$out" | tail -n 3
echo "REPRODUCED: an include inside the project was rejected as outside it"
exit 1
