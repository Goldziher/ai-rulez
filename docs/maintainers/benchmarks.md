# Benchmark gate

This page is for maintainers. It is not in the site navigation. How the gate measures is in
[Performance](../performance.md#regression-tracking).

## What the job does

`.github/workflows/ci.yaml`, job `Benchmarks`: full-history checkout, `task test:benchmark:smoke` (every benchmark
once), then `scripts/bench.sh --compare` with the pull request's base commit (`BENCH_BASE`), or the parent of the
pushed commit. Both revisions are built and measured on the same runner, so a runner with another CPU model never
invalidates the comparison.

## When it fails

- The table names each benchmark, both medians, the change and the p-value. `REGRESSION` rows failed the job.
- Reproduce with `BENCH_COUNT=12 scripts/bench.sh --compare <base>` on a quiet machine. Profile with
  `go test <pkg> -run '^$' -bench <name> -cpuprofile cpu.out`.
- If the slowdown is intended, say why in the pull request and rerun the job once; if it keeps failing and the
  table backs the explanation, a maintainer merges over it. There is no file that silences a benchmark.

## Changing the subset or the limits

The subset is the `SUBSET` list in `scripts/bench.sh`: add only benchmarks that start no process and do not fsync.
`BENCH_THRESHOLD`, `BENCH_ALPHA`, `BENCH_COUNT` and `BENCH_TIME` are environment variables of the step. The fallback
`scripts/bench-baseline.txt` is refreshed with `scripts/bench.sh --update` on the machine that will use it.
