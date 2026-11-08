#!/usr/bin/env bash
# Score a local ai-rulez build against the cached competitors: minutes, no Docker, no competitor rerun.
#
#   tools/skillbench/iterate.sh            # dev track, built from this worktree
#   TRACK=clean tools/skillbench/iterate.sh
#
# TRACK          dev or clean (default dev). The holdout is refused: reading it takes a logged full run.
# AI_RULEZ_BIN   binary to measure (default: built from this worktree).
# WORK           directory for dataset, samples and runs, outside any repository (default
#                $XDG_CACHE_HOME/ai-rulez/skillbench, kept between runs: the download is pinned and verified).
# CACHE_DIR      verdict cache (default bench/skills/cache).
#
# Competitors must already be cached for their pinned versions: run bench.sh once per version.
set -euo pipefail

here="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
TRACK="${TRACK:-dev}"
case "$TRACK" in
dev | clean) ;;
*)
  echo "iterate: track $TRACK reads the holdout; use bench.sh and log the read in bench/skills/HOLDOUT.md" >&2
  exit 2
  ;;
esac
WORK="${WORK:-${XDG_CACHE_HOME:-$HOME/.cache}/ai-rulez/skillbench}"
OUT="${OUT:-$WORK/iterate-$TRACK/out}"
# shellcheck source=tools/skillbench/cache-env.sh
source "$here/cache-env.sh"

tool_bin="$WORK/bin/skillbench"
mkdir -p "$WORK/bin" "$WORK/data" "$OUT"
(cd "$here" && GOWORK=off go build -trimpath -o "$tool_bin" ./cmd/skillbench)
sb() { (cd "$here" && "$tool_bin" "$@"); }

if [ -z "${AI_RULEZ_BIN:-}" ]; then
  AI_RULEZ_BIN="$WORK/bin/ai-rulez"
  # Static, as releases are: a plain go build can leave the binary needing a library from the module cache.
  (cd "$here/../.." && eval "$(bash scripts/setup-liter-llm.sh)" && go build -trimpath -o "$AI_RULEZ_BIN" ./cmd/ai-rulez)
fi

sb fetch --data "$WORK/data"
samples="$WORK/samples-$TRACK"
[ -f "$samples/samples.json" ] || sb materialize --data "$WORK/data" --track "$TRACK" --out "$samples" --every 1

runs="$WORK/iterate-$TRACK/runs"
rm -rf "$runs"
mkdir -p "$runs/iterate"
for tool in ai-rulez-threat ai-rulez; do
  sb run --samples "$samples" --tool "$tool" --bin "$AI_RULEZ_BIN" --max-errors 0 \
    --out "$runs/iterate/$tool-0.jsonl"
done
for tool in skill-scanner skillspector; do
  sb run --samples "$samples" --tool "$tool" --cache "$cache_dir" --tool-version "$(protocol_release "$tool")" \
    --environment "$cache_env" --cached-only --max-errors 5 --out "$runs/iterate/$tool-0.jsonl"
done
printf '{"ai-rulez-threat":"worktree","ai-rulez":"worktree","skill-scanner":"%s","skillspector":"%s"}\n' \
  "$(protocol_release skill-scanner)" "$(protocol_release skillspector)" >"$runs/versions.json"
sb score --samples "$samples/samples.json" --runs "$runs" --versions "$runs/versions.json" --out "$OUT"
