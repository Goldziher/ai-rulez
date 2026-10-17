#!/usr/bin/env bash
# Run the curated, CPU-bound benchmark subset and catch time/op regressions.
#
#   scripts/bench.sh --compare [REF]  build and run the subset at REF (default: the merge base with the
#                                     default branch, or HEAD^ on it) and at HEAD on THIS machine, the two
#                                     interleaved sample by sample, and fail when a benchmark is both slower
#                                     than BENCH_THRESHOLD and different at significance BENCH_ALPHA
#                                     (Mann-Whitney U on the samples, as benchstat does). This is what CI runs.
#   scripts/bench.sh                  run HEAD and compare with the committed scripts/bench-baseline.txt (a
#                                     fallback for a developer machine that recorded one; CI does not use it)
#   scripts/bench.sh --update         run HEAD and rewrite the committed baseline
#
# Environment: BENCH_COUNT (6, at least 5 for --compare), BENCH_THRESHOLD (20, percent slower),
# BENCH_ALPHA (0.05), BENCH_TIME (300ms, -benchtime per sample), BENCH_BASE (the REF of --compare),
# BENCH_BASELINE, BENCH_OUT (benchmark-results.txt), BENCH_TIMEOUT (20m).
#
# --compare measures both revisions on one machine in one run, so neither the CPU model nor a stale file
# matters. A baseline recorded elsewhere is only usable on the same CPU model; otherwise the baseline mode
# warns and exits 0. The pass/fail decision is made here; benchstat, when installed, prints its report too.
set -euo pipefail

cd "$(dirname "$0")/.."

BASELINE="${BENCH_BASELINE:-scripts/bench-baseline.txt}"
OUT="${BENCH_OUT:-benchmark-results.txt}"
COUNT="${BENCH_COUNT:-6}"
THRESHOLD="${BENCH_THRESHOLD:-20}"
ALPHA="${BENCH_ALPHA:-0.05}"
BENCHTIME="${BENCH_TIME:-300ms}"
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
    go test "$pkg" -run '^$' -bench "$regexp" -benchtime="$BENCHTIME" -benchmem -count="$COUNT" -timeout "$TIMEOUT" | tee -a "$OUT"
  done <<<"$SUBSET"
}

cpu_of() { awk -F': ' '/^cpu:/ {gsub(/[ \t]+$/, "", $2); print $2; exit}' "$1"; }

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

# default_base is the revision a branch is measured against: the merge base with the default
# branch, or the parent commit when HEAD is on it.
default_base() {
  local branch ref mb
  branch="$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null || true)"
  for ref in "$branch" origin/main main; do
    [ -n "$ref" ] && git rev-parse --verify --quiet "$ref^{commit}" >/dev/null || continue
    mb="$(git merge-base HEAD "$ref" 2>/dev/null || true)"
    if [ -n "$mb" ] && [ "$mb" != "$(git rev-parse HEAD)" ]; then
      echo "$mb"
      return
    fi
  done
  git rev-parse --verify --quiet 'HEAD^{commit}' >/dev/null && git rev-parse --verify --quiet 'HEAD^' || true
}

# samples_table FILE prints "name ns_per_op" for every sample.
samples_table() {
  awk '/^Benchmark/ && /ns\/op/ {
		name = $1; sub(/-[0-9]+$/, "", name)
		for (i = 2; i <= NF; i++) if ($i == "ns/op") print name, $(i - 1) + 0
	}' "$1"
}

# significance BASE_SAMPLES HEAD_SAMPLES prints one row per benchmark present in both: name, median
# base, median head, delta percent, two-sided Mann-Whitney p-value, verdict; exits 1 on a regression
# (delta over THRESHOLD and p under ALPHA).
significance() {
  awk -v limit="$THRESHOLD" -v alpha="$ALPHA" '
	function erfc_(x,  z, t, r) {
		z = x < 0 ? -x : x; t = 1 / (1 + 0.5 * z)
		r = t * exp(-z * z - 1.26551223 + t * (1.00002368 + t * (0.37409196 + t * (0.09678418 + t * (-0.18628806 + \
			t * (0.27886807 + t * (-1.13520398 + t * (1.48851587 + t * (-0.82215223 + t * 0.17087277)))))))))
		return x >= 0 ? r : 2 - r
	}
	function median(arr, n,  i, j, x, m) {
		for (i = 1; i <= n; i++) tmp[i] = arr[i]
		for (i = 2; i <= n; i++) { x = tmp[i]; j = i - 1; while (j > 0 && tmp[j] > x) { tmp[j + 1] = tmp[j]; j-- } tmp[j + 1] = x }
		m = (n % 2) ? tmp[(n + 1) / 2] : (tmp[n / 2] + tmp[n / 2 + 1]) / 2
		return m
	}
	# pvalue: two-sided Mann-Whitney U, normal approximation with tie and continuity correction.
	function pvalue(a, na, b, nb,  n, i, j, lt, eq, r1, u, mu, ties, sigma2, z, seen) {
		n = na + nb; r1 = 0; ties = 0
		for (i = 1; i <= na; i++) {
			lt = 0; eq = 0
			for (j = 1; j <= na; j++) { if (a[j] < a[i]) lt++; else if (a[j] == a[i]) eq++ }
			for (j = 1; j <= nb; j++) { if (b[j] < a[i]) lt++; else if (b[j] == a[i]) eq++ }
			r1 += lt + (eq + 1) / 2
		}
		for (i = 1; i <= n; i++) {
			v = (i <= na) ? a[i] : b[i - na]
			if (v in seen) continue
			seen[v] = 1; eq = 0
			for (j = 1; j <= na; j++) if (a[j] == v) eq++
			for (j = 1; j <= nb; j++) if (b[j] == v) eq++
			ties += eq * eq * eq - eq
		}
		u = r1 - na * (na + 1) / 2; mu = na * nb / 2
		sigma2 = na * nb / 12 * ((n + 1) - ties / (n * (n - 1)))
		if (sigma2 <= 0) return 1
		z = (((u > mu) ? u - mu : mu - u) - 0.5) / sqrt(sigma2)
		if (z < 0) z = 0
		return erfc_(z / sqrt(2))
	}
	NR == FNR { nb_[$1]++; base[$1, nb_[$1]] = $2 + 0; next }
	{ nh_[$1]++; head[$1, nh_[$1]] = $2 + 0; order[++names] = ($1 in dup) ? "" : $1; dup[$1] = 1 }
	END {
		for (k = 1; k <= names; k++) {
			name = order[k]
			if (name == "" || !(name in nb_)) continue
			na = nb_[name]; nh = nh_[name]
			delete a; delete b
			for (i = 1; i <= na; i++) a[i] = base[name, i]
			for (i = 1; i <= nh; i++) b[i] = head[name, i]
			ma = median(a, na); mh = median(b, nh)
			if (ma <= 0) continue
			delta = (mh - ma) / ma * 100
			p = pvalue(a, na, b, nh)
			verdict = ""
			if (delta > limit && p < alpha) { verdict = "  REGRESSION"; bad++ }
			else if (delta < -limit && p < alpha) verdict = "  faster"
			else if (delta > limit) verdict = "  slower, not significant"
			printf "%-64s %12.0f -> %12.0f ns/op %+7.1f%%  p=%.3f%s\n", name, ma, mh, delta, p, verdict
			compared++
		}
		if (compared == 0) { print "\nno benchmark exists in both revisions; nothing compared"; exit 0 }
		if (bad > 0) { printf "\n%d benchmark(s) are more than %s%% slower with p < %s\n", bad, limit, alpha; exit 1 }
		printf "\nno benchmark is more than %s%% slower with p < %s (%d compared)\n", limit, alpha, compared
	}' "$1" "$2"
}

compare_revisions() {
  local ref="${1:-${BENCH_BASE:-}}" base_sha head_sha work bin n pkg regexp i base_out head_out
  [ -n "$ref" ] || ref="$(default_base)"
  if [ -z "$ref" ] || ! base_sha="$(git rev-parse --verify --quiet "$ref^{commit}")"; then
    echo "::warning::no base revision to compare with (ref '${ref:-none}'; a shallow checkout needs fetch-depth: 0); regression gate skipped"
    return 0
  fi
  head_sha="$(git rev-parse HEAD)"
  if [ "$base_sha" = "$head_sha" ]; then
    echo "::warning::the base revision is HEAD ($head_sha); nothing to compare, regression gate skipped"
    return 0
  fi
  if [ "$COUNT" -lt 5 ]; then
    echo "BENCH_COUNT must be at least 5 for --compare (got $COUNT): fewer samples cannot reach p < $ALPHA" >&2
    return 1
  fi
  work="$(mktemp -d)"
  # shellcheck disable=SC2064 # expand now: the paths are fixed for this run
  trap "git worktree remove --force '$work/base' >/dev/null 2>&1 || true; rm -rf '$work'" EXIT
  git worktree add --detach --quiet "$work/base" "$base_sha"
  mkdir -p "$work/bin"
  base_out="$work/base.txt"
  head_out="${BENCH_OUT}"
  : >"$base_out"
  : >"$head_out"
  echo "comparing base ${base_sha:0:12} with head ${head_sha:0:12}, $COUNT interleaved samples each, benchtime $BENCHTIME" >&2

  n=0
  while read -r pkg regexp; do
    [ -n "$pkg" ] || continue
    n=$((n + 1))
    bin="$work/bin/$n"
    if ! (cd "$work/base" && go test -c -o "$bin.base" "$pkg") >/dev/null 2>"$work/err.txt"; then
      echo "skip $pkg: it does not build at the base revision ($(head -n 1 "$work/err.txt"))" >&2
      continue
    fi
    if ! go test -c -o "$bin.head" "$pkg"; then
      echo "$pkg does not build at HEAD" >&2
      return 1
    fi
    echo "== $pkg $regexp" >&2
    for ((i = 1; i <= COUNT; i++)); do
      # Alternate the order so a warming or throttling machine favors neither revision.
      if ((i % 2)); then sides=(base head); else sides=(head base); fi
      for side in "${sides[@]}"; do
        if [ "$side" = base ]; then
          dir="$work/base/$pkg"
          out="$base_out"
        else
          dir="$pkg"
          out="$head_out"
        fi
        (cd "$dir" && "$bin.$side" -test.run '^$' -test.bench "$regexp" -test.benchtime "$BENCHTIME" -test.benchmem -test.count 1 \
          -test.timeout "$TIMEOUT") >>"$out"
      done
    done
  done <<<"$SUBSET"

  samples_table "$base_out" >"$work/base.samples"
  samples_table "$head_out" >"$work/head.samples"
  significance "$work/base.samples" "$work/head.samples"
}

case "${1:-}" in
--compare)
  compare_revisions "${2:-}"
  exit $?
  ;;
esac

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
