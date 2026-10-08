# Performance

How ai-rulez is benchmarked, how a regression is caught, and what the optimization passes changed.

## Running the benchmarks

Every benchmark that reads a project builds a synthetic one in a temporary directory
(`internal/testutil.BuildBenchTree`): rules, context, skills, agents and commands with frontmatter and prose, plus
a git repository when the code under test asks git questions. Nothing touches the network.

| Size   | Content files | Runs by default |
| ------ | ------------- | --------------- |
| small  | 10            | yes             |
| medium | 200           | yes             |
| large  | 2000          | with `AI_RULEZ_BENCH_LARGE=1` |

```bash
task test:benchmark                 # every benchmark, small and medium
task test:benchmark:large           # adds the 2000-file project, one iteration each
go test ./internal/lint -run '^$' -bench RunFull -benchmem -cpuprofile cpu.out
go tool pprof -top -cum cpu.out
```

| Package | Benchmarks |
| ------- | ---------- |
| `internal/parser` | `ParseFrontmatter` (no/small/rich/CRLF/long body), `ParseFrontmatterBatch` |
| `internal/tokens` | `Estimate`, `Count` (cl100k and byte ratio, prose and long-line input) |
| `internal/config` | `LoadConfig` |
| `internal/includes` | `LoadWithLocalInclude` |
| `internal/lint` | `LoadTree`, `RunFull`, `Analyzers` (each analyzer alone), `SecretScan` (`DetectSecret`, `RedactSecrets`, `ScanText`), `SecretScanLongLine`, `SecretScanCustomPatterns`, `RunSkillTree`, `AnalyzerSelection` |
| `internal/contentlock` | `Compute`, `BuildAndCompare` (lock digest, then verify) |
| `internal/lockfile` | `SaveLoad` |
| `internal/generator` | `Generate` (full and idempotent no-op), `GenerateLLMsTxt`, `Clean` (remove and dry run), `CheckDrift`, `SecretGate` |
| `internal/review` | `Score` (collect and score), `Calibrate` |
| `internal/mcp` | `ProjectTools` (tool dispatch), `SkillServer` (catalog build, `search_skills`, `load_skill`) |
| `internal/verifiers` | `RegexFiles` |
| `cmd/commands` | `Lock` (write and check), `PublishDryRun` (bundle, agent-plugins, ard), `OKF` (export, validate) |

## Regression tracking

`scripts/bench.sh` runs a curated subset (benchmarks that start no process and do not fsync, so they measure the code
and not the machine) with `-count=6` and compares the median time/op of each benchmark with
`scripts/bench-baseline.txt`. It exits 1 when any benchmark is more than 25% slower. When `benchstat` is installed
(or the machine is online, through `go run golang.org/x/perf/cmd/benchstat@latest`) its report is printed too.

```bash
scripts/bench.sh            # run, compare, fail on a >25% regression
scripts/bench.sh --update   # re-record the baseline on this machine
```

Time per operation depends on the CPU, so the gate only applies when the baseline was recorded on the same CPU model
as the run (the `cpu:` line of the output). Otherwise, or when there is no baseline, it prints a warning and exits 0.
The CI `Benchmarks` job runs the script after the full benchmark run and uploads `bench-subset.txt`; to make the gate
bite on the CI runners, commit that artifact as `scripts/bench-baseline.txt`.

## Optimization passes

Numbers are time/op and B/op on an Apple M5 Pro that was running other builds at the same time (load average near
50), taken as alternating runs of test binaries built from the commits below, so the ratios are reliable and the
absolute values are not. Behavior is unchanged: the golden tests and every existing test pass, and each change that
could alter a result has a test comparing it with the code it replaced.

| Path (benchmark) | Change | Before | After |
| ---------------- | ------ | ------ | ----- |
| Credential scan of clean text, 37 KB (`lint SecretScan/DetectSecret/clean/large`) | each credential pattern runs only on text holding one of its literals | 6.1 ms | 0.30 ms (20x) |
| Credential redaction of clean text (`SecretScan/RedactSecrets/clean/large`) | same gate in `secretpat.Redact` and `lint.RedactSecrets` | 6.1 ms, 0.9 MB | 0.28 ms, 0 B |
| Redaction of text holding secrets (`SecretScan/RedactSecrets/dirty/large`) | same gate | 6.1 ms, 0.9 MB | 2.5 ms, 0.33 MB |
| Security scan of one text (`SecretScan/ScanText/clean/large`) | credential gate, then literal gates on the shell-exec (`curl`, `wget`, `base64`, `eval`, `chmod`, `>`) and instruction-override patterns | 29 ms | 7.6 ms (3.8x) |
| Same scan, allocations | ASCII-only lines skip the rune conversion and seen-set of the hidden-character scan | 1.33 MB | 1.18 MB (-11%) |
| Configured secret patterns, 400 lines x 2 patterns (`SecretScanCustomPatterns`) | compiled once per run, not once per scanned line | 12 ms, 7.7 MB | 8 ms, 1.1 MB |
| Lint of 200 content files (`lint RunFull/medium`) | all of the above, tree-relative paths memoized per run, backtick-token prose check only for path-like tokens | 570-660 ms, 53.5 MB, 667k allocs | 265-335 ms, 47.1 MB, 617k allocs |
| Frontmatter parse of a 570 KB document (`parser ParseFrontmatter/longbody`) | find the closing marker by scanning instead of splitting and re-joining the document | 83.6 us, 228 KB | 10.2 us, 7 KB (8x) |
| Frontmatter parse of 200 documents (`ParseFrontmatterBatch/medium`) | same | 1.29 ms, 2.09 MB | 1.0 ms, 1.59 MB |
| Any-file verifier regex, 1000-file tree (`verifiers RegexFiles/medium`) | exclude globs compiled once per predicate, not once per matching file | 13.2 ms, 19.8 MB, 174k allocs | 1.3 ms, 61 KB, 238 allocs (10x) |

The shell and instruction-override pre-checks compare ASCII literals without case in place and treat U+017F and
U+212A as the `s` and `k` the patterns' case folding makes them, so a line the pattern would match always reaches it.

## What dominates elsewhere

These were measured and not changed here.

- **Process spawns per content file.** Loading a project with 200 skills took 91 s on the loaded machine before the
  branch that batches `git ls-files` and `git check-ignore` per directory (0.5 s with it). The remaining
  wall time of `generate`, `clean`, `lock` and `publish` on 200 to 2000 files is mostly git (`rev-parse`,
  `ls-files`) and file-system calls; their CPU time is a small share.
- **Token counting.** cl100k counting of the prompt text is the largest allocator in a lint run (about a third of
  the bytes, in `regexp2` match objects); the memo that removes it is part of the same separate branch.
- **Repeated loads.** `lock --check` and `publish --dry-run` load the configuration three to four times and run the
  security scan again for the served skills. Sharing one load is a larger refactor than this pass.
- **Publish at scale.** `publish --dry-run` on 2000 files allocates about 2.5 GB over 11 million objects.
