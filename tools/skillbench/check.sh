#!/usr/bin/env bash
# Fail if a published table differs from the committed results: the holdout table from the directory named
# by bench/skills/CURRENT, and the unreleased build from bench/skills/dev-unreleased and clean-unreleased.
# RENDER=1 rewrites them instead.
set -euo pipefail
root="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
current="$(cat "$root/bench/skills/CURRENT")"
cd "$root/tools/skillbench"
mode=--check
[ -z "${RENDER:-}" ] || mode=
GOWORK=off go build -o "${TMPDIR:-/tmp}/skillbench-render" ./cmd/skillbench
for pair in "skillbench:$current" skillbench-dev:dev-unreleased skillbench-clean:clean-unreleased; do
  "${TMPDIR:-/tmp}/skillbench-render" render $mode --block "${pair%%:*}" \
    --summary "$root/bench/skills/${pair#*:}/summary.json" \
    "$root/README.md" "$root/website/src/content/docs/benchmarks.md"
done
