#!/usr/bin/env bash
# shellcheck disable=SC2030,SC2031 # phases run in subshells on purpose
# Shared helpers for the corpus harness: logging, result recording, the
# hermetic ai-rulez wrapper and the phase runner. Sourced by run.sh; never
# executed on its own. Written for bash 3.2 (macOS) and newer.

# ---------------------------------------------------------------- logging

log() { printf '%s\n' "$*" >&2; }
die() {
  log "corpus: $*"
  exit 64
}

# ------------------------------------------------------- result recording

# Result of the running phase. Phases call ok_note / fail_note while they work
# and finish (or pass / fail / skip) once; the phase subshell exits afterwards.
PH_FAILS=""
PH_NOTES=""

# _flat collapses newlines and tabs so a reason stays on one line.
_flat() { printf '%s' "$*" | tr '\n\t\r' '   ' | cut -c1-300; }

ok_note() { PH_NOTES="${PH_NOTES:+$PH_NOTES; }$(_flat "$*")"; }
fail_note() { PH_FAILS="${PH_FAILS:+$PH_FAILS; }$(_flat "$*")"; }

# emit prints the one-line verdict and appends it to the results table.
_emit() {
  local status="$1" reason
  reason="$(_flat "$2")"
  printf '%-5s %-18s %-14s %s\n' "$status" "$PHASE" "$REPO_NAME" "$reason"
  printf '%s\t%s\t%s\t%s\t%s\n' "$REPO_NAME" "$PHASE" "$status" "$(date +%s)" "$reason" >>"$RESULTS_TSV"
}

pass() {
  _emit PASS "$*"
  exit 0
}
fail() {
  _emit FAIL "$*"
  exit 0
}
skip() {
  _emit SKIP "$*"
  exit 0
}

# finish turns the collected notes into the verdict.
finish() {
  if [ -n "$PH_FAILS" ]; then
    fail "$PH_FAILS"
  fi
  pass "${1:-$PH_NOTES}"
}

# ------------------------------------------------------------- environment

# have_cmd reports whether a command is on PATH.
have_cmd() { command -v "$1" >/dev/null 2>&1; }

# _timeout_cmd picks a timeout implementation once.
_timeout_cmd() {
  if have_cmd timeout; then
    echo timeout
  elif have_cmd gtimeout; then
    echo gtimeout
  fi
}

# setup_env builds the sandbox the binary and git run in: a private HOME, no
# credentials, and a dead proxy so an accidental network call fails fast
# instead of hanging.
setup_env() {
  CORPUS_HOME="$WORK/home"
  mkdir -p "$CORPUS_HOME/.config" "$CORPUS_HOME/.cache" "$CORPUS_HOME/.state"
  cat >"$CORPUS_HOME/.gitconfig" <<'EOF'
[user]
	name = corpus
	email = corpus@example.invalid
[commit]
	gpgsign = false
[tag]
	gpgsign = false
[init]
	defaultBranch = main
[protocol "file"]
	allow = always
EOF
  TIMEOUT_BIN="$(_timeout_cmd)"
  CORPUS_TIMEOUT="${CORPUS_TIMEOUT:-300}"
  sandbox_env
}

# sandbox_env fills SB_ENV with the variables of the clean environment.
sandbox_env() {
  SB_ENV=(
    "PATH=$PATH"
    "HOME=$CORPUS_HOME"
    "XDG_CONFIG_HOME=$CORPUS_HOME/.config"
    "XDG_CACHE_HOME=$CORPUS_HOME/.cache"
    "XDG_STATE_HOME=$CORPUS_HOME/.state"
    "TMPDIR=$WORK/tmp"
    "TERM=dumb"
    "NO_COLOR=1"
    "CI=1"
    "LC_ALL=C"
    "SOURCE_DATE_EPOCH=1700000000"
    "GIT_TERMINAL_PROMPT=0"
    "GIT_OPTIONAL_LOCKS=0"
    "GIT_CONFIG_NOSYSTEM=1"
    "GIT_ASKPASS=/bin/false"
    "SSH_ASKPASS=/bin/false"
    "GIT_SSH_COMMAND=false"
    "HTTP_PROXY=http://127.0.0.1:9"
    "HTTPS_PROXY=http://127.0.0.1:9"
    "ALL_PROXY=http://127.0.0.1:9"
    "NO_PROXY="
    "AI_RULEZ_REVIEWER=corpus@example.invalid"
  )
}

# run_clean runs a command in the sandbox environment with the phase timeout.
# Usage: run_clean <logfile> <cmd> [args...]; sets RC.
run_clean() {
  local log_file="$1"
  shift
  if [ -n "$TIMEOUT_BIN" ]; then
    env -i "${SB_ENV[@]}" "$TIMEOUT_BIN" "$CORPUS_TIMEOUT" "$@" <"${AR_STDIN:-/dev/null}" >"$log_file" 2>&1
  else
    env -i "${SB_ENV[@]}" "$@" <"${AR_STDIN:-/dev/null}" >"$log_file" 2>&1
  fi
  RC=$?
}

# run_split is run_clean with standard output in its own file.
# Usage: run_split <stdout-file> <stderr-file> <cmd> [args...]; sets RC.
run_split() {
  local out_file="$1" err_file="$2"
  shift 2
  if [ -n "$TIMEOUT_BIN" ]; then
    env -i "${SB_ENV[@]}" "$TIMEOUT_BIN" "$CORPUS_TIMEOUT" "$@" <"${AR_STDIN:-/dev/null}" >"$out_file" 2>"$err_file"
  else
    env -i "${SB_ENV[@]}" "$@" <"${AR_STDIN:-/dev/null}" >"$out_file" 2>"$err_file"
  fi
  RC=$?
}

# sgit runs git in the sandbox environment, quietly. Only ever used inside the
# throwaway copies and the snapshot, never in a source repository.
sgit() {
  env -i "${SB_ENV[@]}" git "$@" >/dev/null 2>&1
}

# ar runs the binary under test in $PWD. Output goes to a numbered log; AR_LOG
# names it and AR_RC holds the exit status. A panic, a fatal runtime error or a
# timeout is recorded as a failure of the phase whatever the exit status was.
# Usage: ar <args...>
ar() {
  PH_SEQ=$((PH_SEQ + 1))
  AR_LOG="$PH_LOGS/$(printf '%02d' "$PH_SEQ").log"
  {
    printf '$ ai-rulez'
    printf ' %s' "$@"
    printf '\n'
  } >"$AR_LOG.cmd"
  run_clean "$AR_LOG" "$BIN" "$@"
  AR_RC=$RC
  if [ "$AR_RC" -eq 124 ] && [ -n "$TIMEOUT_BIN" ]; then
    fail_note "timeout after ${CORPUS_TIMEOUT}s: ai-rulez $*"
  fi
  if grep -qE '^(panic: |fatal error: |goroutine [0-9]+ \[)' "$AR_LOG"; then
    fail_note "panic in: ai-rulez $*"
  fi
  return 0
}

# ar_to is ar with standard output captured in its own file, for commands whose
# stdout is a document. Usage: ar_to <file> <args...>
ar_to() {
  local out_file="$1"
  shift
  PH_SEQ=$((PH_SEQ + 1))
  AR_LOG="$PH_LOGS/$(printf '%02d' "$PH_SEQ").log"
  {
    printf '$ ai-rulez'
    printf ' %s' "$@"
    printf ' > %s\n' "$out_file"
  } >"$AR_LOG.cmd"
  run_split "$out_file" "$AR_LOG" "$BIN" "$@"
  AR_RC=$RC
  if [ "$AR_RC" -eq 124 ] && [ -n "$TIMEOUT_BIN" ]; then
    fail_note "timeout after ${CORPUS_TIMEOUT}s: ai-rulez $*"
  fi
  if grep -qE '^(panic: |fatal error: |goroutine [0-9]+ \[)' "$AR_LOG"; then
    fail_note "panic in: ai-rulez $*"
  fi
  return 0
}

# ar_in runs ar inside a directory. Usage: ar_in <dir> <args...>
ar_in() {
  local dir="$1" old="$PWD"
  shift
  cd "$dir" || return 1
  ar "$@"
  cd "$old" || return 1
}

# last_msg prints the last meaningful line of the latest log, for reasons.
last_msg() {
  grep -vE '^\s*$' "${1:-$AR_LOG}" 2>/dev/null | tail -n 1 | cut -c1-160
}

# expect_rc checks the exit status of the last ar call.
# Usage: expect_rc <want> <label>
expect_rc() {
  if [ "$AR_RC" -ne "$1" ]; then
    fail_note "$2: exit $AR_RC, want $1 ($(last_msg))"
    return 1
  fi
  return 0
}

# expect_rc_in checks the exit status against a list. Usage: <label> <rc...>
expect_rc_in() {
  local label="$1" want
  shift
  for want in "$@"; do
    [ "$AR_RC" -eq "$want" ] && return 0
  done
  fail_note "$label: exit $AR_RC, want one of $* ($(last_msg))"
  return 1
}

# skip_if_gated ends the phase as SKIP when the last command was refused by the
# validation gate (exit 2, "validate reported findings"): the repository's own
# configuration has error-level findings, which says nothing about the phase.
skip_if_gated() {
  if [ "${AR_RC:-0}" -eq 2 ] && grep -q 'validate reported findings' "$AR_LOG" 2>/dev/null; then
    skip "$1 refused by the repository's own validation findings"
  fi
}

# expect_grep checks that a pattern occurs in a file. Usage: <pattern> <file> <label>
expect_grep() {
  if ! grep -qE -- "$1" "$2" 2>/dev/null; then
    fail_note "$3: pattern not found in ${2##*/}"
    return 1
  fi
  return 0
}

# --------------------------------------------------------------- phase runner

# list_phases prints every phase name defined by a phase_<name> function.
list_phases() {
  declare -F | awk '{print $3}' | sed -n 's/^phase_//p' | sort
}

# run_phase runs one phase for one repo in its own subshell and fresh copy.
# Usage: run_phase <phase> <repo-id> <repo-name>
run_phase() {
  local phase="$1" repo_id="$2" name="$3" started
  started=$(date +%s)
  (
    PHASE="$phase"
    REPO_NAME="$name"
    PH_SEQ=0
    PH_LOGS="$OUT/logs/$repo_id/$phase"
    mkdir -p "$PH_LOGS"
    W="$RUN/$repo_id/$phase"
    SNAP_DIR="$SNAP/$repo_id"
    cd "$WORK" || exit 1
    "phase_$phase"
    # A phase that returns without a verdict is a harness bug.
    fail "phase_$phase returned without a verdict"
  )
  local elapsed=$(($(date +%s) - started))
  printf '%s\t%s\t%s\n' "$name" "$phase" "$elapsed" >>"$TIMES_TSV"
  rm -rf "${RUN:?}/$repo_id/$phase"
}

# fresh_copy makes the phase's private working tree from the snapshot, using
# copy-on-write where the filesystem has it.
fresh_copy() {
  local dest="${1:-$W}"
  mkdir -p "$(dirname "$dest")"
  if cp -Rc "$SNAP_DIR" "$dest" 2>/dev/null; then
    return 0
  fi
  rm -rf "${dest:?}"
  if cp -R --reflink=auto "$SNAP_DIR" "$dest" 2>/dev/null; then
    return 0
  fi
  rm -rf "${dest:?}"
  cp -R "$SNAP_DIR" "$dest"
}
