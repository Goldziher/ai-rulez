#!/usr/bin/env bash
# Corpus harness: runs the corpus phases of a built ai-rulez binary against a
# list of local git repositories and writes a PASS/FAIL/SKIP table plus a
# machine-readable summary. See README.md.
#
# Source repositories are strictly read-only: each is copied once with
# `git archive`, every phase works in a private copy under $TMPDIR, and the
# HEAD and working-tree status of each source are compared before and after.
set -u

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
	cat <<'EOF'
usage: run.sh [options] BINARY [REPOS_FILE]

  BINARY       path to the ai-rulez binary under test
  REPOS_FILE   file with one repository path per line (# starts a comment);
               without it the paths come from $CORPUS_REPOS (whitespace or
               newline separated). There is no built-in list.

options:
  -o DIR       output directory for summary.json, results.tsv and logs
               (default: a fresh directory under $TMPDIR)
  -p LIST      run only these phases (comma separated)
  -x LIST      leave these phases out (comma separated)
  -l           list the phases and exit
  -k           keep the working directory (copies and snapshots) after the run
  -h           show this help

environment:
  CORPUS_REPOS            repository paths, see above
  CORPUS_TIMEOUT          seconds per ai-rulez invocation (default 300)
  CORPUS_ARCHIVE_EXCLUDE  git pathspecs (space separated) to leave out of the
                          snapshot, for repositories with huge fixture trees
EOF
}

OUT=""
ONLY=""
SKIPLIST=""
KEEP=0
LIST=0
while getopts "o:p:x:lkh" opt; do
	case "$opt" in
	o) OUT="$OPTARG" ;;
	p) ONLY="$OPTARG" ;;
	x) SKIPLIST="$OPTARG" ;;
	l) LIST=1 ;;
	k) KEEP=1 ;;
	h)
		usage
		exit 0
		;;
	*)
		usage >&2
		exit 64
		;;
	esac
done
shift $((OPTIND - 1))

# shellcheck source=lib/common.sh
. "$HERE/lib/common.sh"
# shellcheck source=lib/repo.sh
. "$HERE/lib/repo.sh"
for f in "$HERE"/lib/phases_*.sh; do
	# shellcheck disable=SC1090
	. "$f"
done

if [ "$LIST" -eq 1 ]; then
	list_phases
	exit 0
fi

[ $# -ge 1 ] || {
	usage >&2
	exit 64
}
BIN="$1"
REPOS_FILE="${2:-}"
case "$BIN" in
/*) ;;
*) BIN="$PWD/$BIN" ;;
esac
[ -x "$BIN" ] || die "binary not found or not executable: $BIN"
have_cmd jq || die "jq is required"
have_cmd git || die "git is required"

# Collect the repository list.
REPO_PATHS=()
if [ -n "$REPOS_FILE" ]; then
	[ -f "$REPOS_FILE" ] || die "repository list not found: $REPOS_FILE"
	while IFS= read -r line; do
		line="${line%%#*}"
		line="$(printf '%s' "$line" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
		[ -n "$line" ] && REPO_PATHS+=("$line")
	done <"$REPOS_FILE"
elif [ -n "${CORPUS_REPOS:-}" ]; then
	for p in $CORPUS_REPOS; do
		REPO_PATHS+=("$p")
	done
fi
[ "${#REPO_PATHS[@]}" -gt 0 ] || die "no repositories: pass a list file or set CORPUS_REPOS (there is no default list)"

# Phase selection.
ALL_PHASES="$(list_phases)"
PHASES=()
ordered="upgrade mcp_smoke plugin skipped_content agent_conventions sparse local_includes profile role recursive repaired frozen signing determinism injected offline_tools validate_formats agent_plugins ard llms_txt"
for p in $ordered; do
	printf '%s\n' "$ALL_PHASES" | grep -qx "$p" || continue
	if [ -n "$ONLY" ]; then
		case ",$ONLY," in *",$p,"*) ;; *) continue ;; esac
	fi
	case ",$SKIPLIST," in *",$p,"*) continue ;; esac
	PHASES+=("$p")
done
if [ -n "$ONLY" ]; then
	IFS=',' read -r -a want <<<"$ONLY"
	for p in "${want[@]}"; do
		printf '%s\n' "$ALL_PHASES" | grep -qx "$p" || die "unknown phase: $p (see -l)"
	done
fi
[ "${#PHASES[@]}" -gt 0 ] || die "no phase selected"

# Directories.
WORK="$(mktemp -d "${TMPDIR:-/tmp}/corpus-XXXXXX")" || die "cannot create a working directory"
case "$WORK" in */corpus-*) ;; *) die "unexpected working directory: $WORK" ;; esac
SNAP="$WORK/snap"
RUN="$WORK/run"
mkdir -p "$SNAP" "$RUN" "$WORK/tmp"
if [ -z "$OUT" ]; then
	OUT="$(mktemp -d "${TMPDIR:-/tmp}/corpus-out-XXXXXX")"
fi
mkdir -p "$OUT/logs"
RESULTS_TSV="$OUT/results.tsv"
TIMES_TSV="$WORK/times.tsv"
: >"$RESULTS_TSV"
: >"$TIMES_TSV"

# shellcheck disable=SC2329 # invoked by the EXIT trap
cleanup() {
	if [ "$KEEP" -eq 1 ]; then
		log "corpus: kept $WORK"
	else
		rm -rf "${WORK:?}"
	fi
}
trap cleanup EXIT

setup_env
export GIT_OPTIONAL_LOCKS=0

BIN_VERSION="$(run_clean "$WORK/version.txt" "$BIN" version && head -n 1 "$WORK/version.txt")"

log "corpus: binary $BIN ($BIN_VERSION)"
log "corpus: output $OUT"

SRC_REPORT="$WORK/sources.tsv"
: >"$SRC_REPORT"
idx=0
for src in "${REPO_PATHS[@]}"; do
	name="$(basename "$src")"
	if ! GIT_OPTIONAL_LOCKS=0 git -C "$src" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
		log "corpus: skipping $name: not a git repository (or missing)"
		printf '%s\t%s\t%s\n' "$name" "missing" "-" >>"$SRC_REPORT"
		continue
	fi
	idx=$((idx + 1))
	id="r$idx"
	before="$(source_fingerprint "$src")"
	log "corpus: snapshot $name"
	if ! snapshot_repo "$src" "$id"; then
		log "corpus: snapshot of $name failed"
		printf '%s\t%s\t%s\n' "$name" "snapshot-failed" "-" >>"$SRC_REPORT"
		continue
	fi
	for phase in "${PHASES[@]}"; do
		run_phase "$phase" "$id" "$name"
	done
	after="$(source_fingerprint "$src")"
	if [ "$before" = "$after" ]; then
		printf '%s\t%s\t%s\n' "$name" "untouched" "$id" >>"$SRC_REPORT"
	else
		printf '%s\t%s\t%s\n' "$name" "MODIFIED" "$id" >>"$SRC_REPORT"
		log "corpus: SOURCE CHANGED during the run: $name"
	fi
	rm -rf "${SNAP:?}/$id"
done

# Summary.
phases_json="$(printf '%s\n' "${PHASES[@]}" | jq -R . | jq -s .)"
jq -n \
	--arg binary "$BIN" \
	--arg version "$BIN_VERSION" \
	--argjson phases "$phases_json" \
	--rawfile results "$RESULTS_TSV" \
	--rawfile times "$TIMES_TSV" \
	--rawfile sources "$SRC_REPORT" '
	def rows(s): s | split("\n") | map(select(length > 0) | split("\t"));
	(rows($times) | map({key: (.[0] + "|" + .[1]), value: (.[2] | tonumber)}) | from_entries) as $secs
	| (rows($results) | map({
			repo: .[0], phase: .[1], status: .[2],
			seconds: ($secs[.[0] + "|" + .[1]] // 0),
			reason: .[4]
		})) as $r
	| (rows($sources) | map({repo: .[0], state: .[1]})) as $s
	| {
		schema_version: 1,
		binary_version: $version,
		phases: $phases,
		sources: $s,
		sources_untouched: ($s | all(.state == "untouched" or .state == "missing")),
		totals: {
			PASS: ($r | map(select(.status == "PASS")) | length),
			FAIL: ($r | map(select(.status == "FAIL")) | length),
			SKIP: ($r | map(select(.status == "SKIP")) | length)
		},
		by_phase: ($phases | map(. as $p | {
			phase: $p,
			PASS: ($r | map(select(.phase == $p and .status == "PASS")) | length),
			FAIL: ($r | map(select(.phase == $p and .status == "FAIL")) | length),
			SKIP: ($r | map(select(.phase == $p and .status == "SKIP")) | length)
		})),
		results: $r
	}' </dev/null >"$OUT/summary.json" || log "corpus: could not write summary.json"

pass_n="$(jq '.totals.PASS' "$OUT/summary.json" 2>/dev/null || echo "?")"
fail_n="$(jq '.totals.FAIL' "$OUT/summary.json" 2>/dev/null || echo "?")"
skip_n="$(jq '.totals.SKIP' "$OUT/summary.json" 2>/dev/null || echo "?")"
log ""
log "corpus: PASS=$pass_n FAIL=$fail_n SKIP=$skip_n  summary: $OUT/summary.json"
if grep -q MODIFIED "$SRC_REPORT"; then
	exit 3
fi
[ "$fail_n" = "0" ] || exit 1
exit 0
