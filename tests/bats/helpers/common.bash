# shellcheck shell=bash
# Helpers shared by the ai-rulez Bats suites. Load with `load "${BATS_TEST_DIRNAME}/../helpers/common"`
# after `bats_load_library xberg-bats`.

# The repository root: the nearest ancestor of the suite that holds go.mod.
repo_root() {
  local dir="$BATS_TEST_DIRNAME"
  while [ "$dir" != "/" ]; do
    [ -f "$dir/go.mod" ] && {
      printf '%s' "$dir"
      return 0
    }
    dir="$(dirname "$dir")"
  done
  printf 'repo_root: no go.mod above %s\n' "$BATS_TEST_DIRNAME" >&2
  return 1
}

# Point HOME and every XDG directory below $1 (default the test's temp dir), so nothing an
# ai-rulez run writes for the user (caches, telemetry state, user config) reaches the developer's
# real home. setup_file passes $BATS_FILE_TMPDIR.
isolate_home() {
  export HOME="${1:-$BATS_TEST_TMPDIR}/home"
  export XDG_CONFIG_HOME="$HOME/.config"
  export XDG_CACHE_HOME="$HOME/.cache"
  export XDG_DATA_HOME="$HOME/.local/share"
  export XDG_STATE_HOME="$HOME/.local/state"
  export GIT_CONFIG_NOSYSTEM=1
  mkdir -p "$XDG_CONFIG_HOME" "$XDG_CACHE_HOME" "$XDG_DATA_HOME" "$XDG_STATE_HOME"
}

# Print the path of an ai-rulez binary built from this checkout.
#
# scripts/bats/run.sh builds it once and exports AI_RULEZ_BATS_BIN; a suite run directly with
# `bats` builds into the run's temp dir instead, so the binary under test is never a stale one
# from PATH.
ai_rulez_bin() {
  if [ -n "${AI_RULEZ_BATS_BIN:-}" ]; then
    [ -x "$AI_RULEZ_BATS_BIN" ] || {
      printf 'AI_RULEZ_BATS_BIN is not executable: %s\n' "$AI_RULEZ_BATS_BIN" >&2
      return 1
    }
    printf '%s' "$AI_RULEZ_BATS_BIN"
    return 0
  fi
  local bin="$BATS_RUN_TMPDIR/ai-rulez"
  if [ ! -x "$bin" ]; then
    (cd "$(repo_root)" && go build -o "$bin" ./cmd/ai-rulez) >&2 || return 1
  fi
  printf '%s' "$bin"
}

# Write a self-contained stub into $XBERG_STUB_BIN, replacing xberg-bats' xberg_stub for suites
# that must be certain no host binary runs.
#
# xberg_stub gives its stub a `#!/usr/bin/env bash` shebang, so the kernel starts the stub
# through /usr/bin/env, which re-resolves `bash` on PATH at exec time. A stub directory that is
# only prepended therefore leaves a window where a hook's `npx`/`npm` lookup reaches the host's
# node-based copy: one CI run of the Husky suite died with SIGSEGV (exit 139) inside a stubbed
# npx this way. This stub names /bin/sh in the shebang (no PATH lookup starts it) and its body
# is shell builtins only, so it can never exec node or a host npx/npm.
ai_rulez_write_stub() {
  local name="$1" status="$2" label="$3"
  {
    printf '%s\n' '#!/bin/sh'
    if [ "$status" -eq 0 ]; then
      # shellcheck disable=SC2016  # the stub body must expand when the STUB runs, not now
      printf 'printf "%%s %%s\\n" "%s" "$*" >>"$XBERG_TRACE"\n' "$label"
    fi
    printf 'exit %s\n' "$status"
  } >"$XBERG_STUB_BIN/$name"
  chmod +x "$XBERG_STUB_BIN/$name"
}

# A self-contained stub that traces its invocation to $XBERG_TRACE, then exits 0: the
# xberg_stub_trace shape without xberg_stub's PATH-resolving shebang.
ai_rulez_stub_trace() {
  local name="$1" label="${2:-$1}"
  ai_rulez_write_stub "$name" 0 "$label"
}

# A self-contained stub that exits with the given status and traces nothing: the xberg_stub_exit
# shape.
ai_rulez_stub_exit() {
  ai_rulez_write_stub "$1" "${2:-0}" "$1"
}

# Put $XBERG_STUB_BIN first on PATH and prove the command resolves to its stub. Call it once the
# stubs exist and before running a hook that invokes npx/npm, so a host binary can never win the
# lookup.
ai_rulez_stub_first() {
  local name="$1" found
  export PATH="$XBERG_STUB_BIN:$PATH"
  hash -r 2>/dev/null || true
  found="$(command -v "$name" || true)"
  [ "$found" = "$XBERG_STUB_BIN/$name" ] || {
    printf 'ai_rulez_stub_first: %s resolves to <%s>, not the stub <%s>\n' \
      "$name" "${found:-nothing}" "$XBERG_STUB_BIN/$name" >&2
    return 1
  }
}

# Create a scratch project at $1 holding a minimal V4 config with the presets list $2 (already
# quoted, e.g. '"cline"') and the optional TOML $3 appended. The project is a git repository so generate behaves as it does in
# a real checkout.
make_project() {
  local dir="$1" presets="$2" extra="${3:-}"
  mkdir -p "$dir/.ai-rulez"
  {
    printf 'version = "5.0"\nname = "bats"\npresets = [%s]\n' "$presets"
    [ -z "$extra" ] || printf '%s\n' "$extra"
  } >"$dir/.ai-rulez/config.toml"
  git -C "$dir" init -q
}

# Skip a test that demonstrates an open bug in code this suite does not own, unless
# AI_RULEZ_BATS_KNOWN_BUGS=1 (then it runs and fails until the bug is fixed). The fix removes the
# call. The reason names the file and the defect.
known_bug() {
  [ "${AI_RULEZ_BATS_KNOWN_BUGS:-0}" = 1 ] || skip "known bug: $1"
}
