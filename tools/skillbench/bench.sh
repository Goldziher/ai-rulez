#!/usr/bin/env bash
# Run the benchmark end to end, the same way locally and in CI.
#
#   TRACK=test ALLOW_HOLDOUT=1 tools/skillbench/bench.sh
#
# TRACK          test, dev, full or clean (default dev). test and full read the holdout: log it first.
# EVERY          keep one sample in N, dev only (default 1).
# REPS           repetitions (default 3).
# SHARD/SHARDS   this shard of the population (default 0 of 1). CI runs several and scores them.
# PHASE          all, or scan or score to split the two across CI jobs (default all).
# WORK           directory for dataset, samples and runs, outside any repository (default a mktemp).
# OUT            directory for the published files (default $WORK/out).
# AI_RULEZ_VERSION  release to measure (default: the protocol's).
# NETWORK        none, or bridge to compare a networked run (default none).
# CACHE_DIR      verdict cache for Cisco and SkillSpector (default bench/skills/cache). See iterate.sh.
# VERIFY         cached samples to rescan per tool: a moved verdict fails the run (default 25).
# REFRESH        set to 1 to ignore the cache and rescan everything.
set -euo pipefail

here="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
TRACK="${TRACK:-dev}"
EVERY="${EVERY:-1}"
REPS="${REPS:-3}"
SHARD="${SHARD:-0}"
SHARDS="${SHARDS:-1}"
PHASE="${PHASE:-all}"
NETWORK="${NETWORK:-none}"
VERIFY="${VERIFY:-25}"
WORK="${WORK:-$(mktemp -d "${TMPDIR:-/tmp}/skillbench.XXXXXX")}"
OUT="${OUT:-$WORK/out}"
AI_RULEZ_VERSION="${AI_RULEZ_VERSION:-$(awk '/^\[scanners.ai-rulez\]/{f=1;next} f&&/^release/{gsub(/"/,"",$3);print $3;exit}' "$here/protocol.toml")}"
image="skillbench:${AI_RULEZ_VERSION}"
# shellcheck source=tools/skillbench/cache-env.sh
source "$here/cache-env.sh"
arch="$(uname -m)"
tool_bin="$WORK/bin/skillbench"

mkdir -p "$WORK/bin" "$WORK/data" "$OUT"
(cd "$here" && GOWORK=off go build -trimpath -o "$tool_bin" ./cmd/skillbench)
sb() { (cd "$here" && "$tool_bin" "$@"); }

sb fetch --data "$WORK/data"
samples="$WORK/samples-$TRACK"
if [ ! -f "$samples/samples.json" ]; then
  args=(--data "$WORK/data" --track "$TRACK" --out "$samples" --every "$EVERY")
  [ -n "${ALLOW_HOLDOUT:-}" ] && args+=(--allow-holdout)
  sb materialize "${args[@]}"
fi

if [ "$PHASE" != score ]; then
  docker build --quiet --build-arg "AI_RULEZ_VERSION=$AI_RULEZ_VERSION" -t "$image" "$here" >/dev/null
  in_image() {
    docker run --rm --network "$NETWORK" --read-only --tmpfs /tmp:rw,mode=1777 --cap-drop ALL \
      --security-opt no-new-privileges --user "$(id -u):$(id -g)" -e TMPDIR=/tmp \
      -v "$samples:/samples:ro" -v "$WORK/runs:/runs" -v "$cache_dir:/cache" "$image" "$@"
  }
  mkdir -p "$WORK/runs" "$cache_dir"
  [ -z "${REFRESH:-}" ] || rm -f "$cache_dir"/skill-scanner-*.jsonl "$cache_dir"/skillspector-*.jsonl
  in_image versions --out /runs/versions.json
  for rep in $(seq 1 "$REPS"); do
    run="$arch-$NETWORK-rep$rep"
    mkdir -p "$WORK/runs/$run"
    for tool in ai-rulez-threat ai-rulez; do
      in_image run --samples /samples --tool "$tool" --shard "$SHARD" --shards "$SHARDS" --max-errors 0 \
        --out "/runs/$run/$tool-$SHARD.jsonl"
    done
    for tool in skill-scanner skillspector; do
      in_image run --samples /samples --tool "$tool" --shard "$SHARD" --shards "$SHARDS" \
        --cache /cache --tool-version "$(protocol_release "$tool")" --environment "$cache_env" --verify "$VERIFY" --max-errors 25 \
        --out "/runs/$run/$tool-$SHARD.jsonl"
    done
  done
fi

if [ "$PHASE" != scan ]; then
  sb score --samples "$samples/samples.json" --runs "$WORK/runs" --versions "$WORK/runs/versions.json" --out "$OUT"
  cp "$samples/samples.json" "$OUT/samples.json"
  cp "$WORK/runs/versions.json" "$OUT/versions.json"
  runs="$(cd "$WORK/runs" && find . -mindepth 1 -maxdepth 1 -type d | sed -e 's|^\./||' -e 's/.*/"&"/' | LC_ALL=C sort | paste -sd, -)"
  printf '{"commit":"%s","runs":[%s]}\n' "$(git -C "$here" rev-parse HEAD)" "$runs" >"$OUT/manifest.json"
fi
