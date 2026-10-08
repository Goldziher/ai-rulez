#!/usr/bin/env bash
# Run the curated, CPU-bound benchmark subset and compare it with the checked-in
# baseline (scripts/bench-baseline.txt).
#
#   scripts/bench.sh            run (-count=6), compare, exit 1 on a >25% time/op regression
#   scripts/bench.sh --update   run and rewrite the baseline
#
# Environment: BENCH_COUNT (6), BENCH_THRESHOLD (25, percent), BENCH_BASELINE,
# BENCH_OUT (benchmark-results.txt), BENCH_TIMEOUT (20m).
#
# time/op depends on the machine, so the gate only applies when the baseline was
# recorded on the same CPU model; otherwise it warns and exits 0. A missing
# baseline also only warns. benchstat, when installed, prints its comparison for
# reading; the pass/fail decision is made here on the median of the runs.
set -euo pipefail

cd "$(dirname "$0")/.."

BASELINE="${BENCH_BASELINE:-scripts/bench-baseline.txt}"
OUT="${BENCH_OUT:-benchmark-results.txt}"
COUNT="${BENCH_COUNT:-6}"
THRESHOLD="${BENCH_THRESHOLD:-25}"
TIMEOUT="${BENCH_TIMEOUT:-20m}"

# One "package regexp" pair per line. Only benchmarks that start no process and
# do not fsync: those measure the machine, not the code.
SUBSET='
./internal/parser      ^BenchmarkParseFrontmatter$
./internal/tokens      ^(BenchmarkEstimate|BenchmarkCount)$/^(ratio|cl100k/prose-small)
./internal/lint        ^BenchmarkSecretScan$
./internal/lint        ^BenchmarkRunFull$/^small$
./internal/contentlock ^BenchmarkBuildAndCompare$
./internal/lockfile    ^BenchmarkSaveLoad$/^Load
./internal/review      ^BenchmarkScore$/^Run
./internal/review      ^BenchmarkCalibrate$
./internal/mcp         ^BenchmarkSkillServer$/^BuildCatalog
'

run_subset() {
  : >"$OUT"
  while read -r pkg regexp; do
    [ -n "$pkg" ] || continue
    echo "== $pkg $regexp" >&2
    go test "$pkg" -run '^$' -bench "$regexp" -benchmem -count="$COUNT" -timeout "$TIMEOUT" | tee -a "$OUT"
  done <<<"$SUBSET"
}

cpu_of() { awk -F': ' '/^cpu:/ {print $2; exit}' "$1"; }

# median_table FILE prints "name median_ns" lines.
median_table() {
  awk '
		/^Benchmark/ && /ns\/op/ {
			name = $1; sub(/-[0-9]+$/, "", name)
			for (i = 2; i <= NF; i++) if ($i == "ns/op") { n[name]++; v[name, n[name]] = $(i - 1) + 0 }
		}
		END {
			for (name in n) {
				c = n[name]
				for (i = 1; i <= c; i++) a[i] = v[name, i]
				for (i = 2; i <= c; i++) { x = a[i]; j = i - 1; while (j > 0 && a[j] > x) { a[j + 1] = a[j]; j-- } a[j + 1] = x }
				m = (c % 2) ? a[(c + 1) / 2] : (a[c / 2] + a[c / 2 + 1]) / 2
				printf "%s %.2f\n", name, m
			}
		}' "$1" | sort
}

show_benchstat() {
  if command -v benchstat >/dev/null 2>&1; then
    benchstat "$BASELINE" "$OUT" || true
  elif command -v curl >/dev/null 2>&1 && curl -fsS --max-time 3 -o /dev/null https://proxy.golang.org; then
    go run golang.org/x/perf/cmd/benchstat@latest "$BASELINE" "$OUT" || true
  else
    echo "benchstat not installed and offline: skipping its report" >&2
  fi
}

run_subset

if [ "${1:-}" = "--update" ]; then
  cp "$OUT" "$BASELINE"
  echo "baseline written to $BASELINE (cpu: $(cpu_of "$BASELINE"))"
  exit 0
fi

if [ ! -s "$BASELINE" ]; then
  echo "::warning::no benchmark baseline at $BASELINE; run scripts/bench.sh --update to record one"
  exit 0
fi
if [ "$(cpu_of "$BASELINE")" != "$(cpu_of "$OUT")" ]; then
  echo "::warning::baseline was recorded on '$(cpu_of "$BASELINE")' and this run is on '$(cpu_of "$OUT")'; time/op is not comparable, regression gate skipped"
  exit 0
fi

show_benchstat

base_tmp="$(mktemp)"
new_tmp="$(mktemp)"
trap 'rm -f "$base_tmp" "$new_tmp"' EXIT
median_table "$BASELINE" >"$base_tmp"
median_table "$OUT" >"$new_tmp"

awk -v limit="$THRESHOLD" '
	NR == FNR { base[$1] = $2; next }
	($1 in base) && base[$1] > 0 {
		delta = ($2 - base[$1]) / base[$1] * 100
		flag = (delta > limit) ? "  REGRESSION" : ""
		printf "%-70s %12.0f -> %12.0f ns/op %+7.1f%%%s\n", $1, base[$1], $2, delta, flag
		if (delta > limit) bad++
	}
	END {
		if (bad > 0) { printf "\n%d benchmark(s) regressed by more than %s%%\n", bad, limit; exit 1 }
		print "\nno benchmark regressed by more than " limit "%"
	}' "$base_tmp" "$new_tmp"
