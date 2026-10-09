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
| `internal/lint` | `LoadTree`, `RunFull`, `Analyzers` (each analyzer alone), `SecretScan` (`DetectSecret`, `RedactSecrets`, `ScanText`), `SecretScanLongLine`, `SecretScanCustomPatterns`, `RunSkillTree`, `AnalyzerSelection`, `NumberedPrefixExists` |
| `internal/contentlock` | `Compute`, `BuildAndCompare` (lock digest, then verify) |
| `internal/lockfile` | `SaveLoad` |
| `internal/generator` | `Generate` (full and idempotent no-op), `GenerateLLMsTxt`, `Clean` (remove and dry run), `CheckDrift`, `SecretGate` (plain and `-memo`, the context `generate` runs under) |
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

### Second pass: repeated loads, publish at scale, startup

Same method as above, taken on a quieter machine (load average near 20): test binaries built from the commit before
the pass and from the last commit of it, run back to back with `-benchtime=3x -count=2` (`1x` for the 2000-file
project), so the ratios are reliable and the absolute values are not. Output is unchanged: `tests/golden` passes
without edits.

| Path (benchmark) | Change | Before | After |
| ---------------- | ------ | ------ | ----- |
| `lock --check`, 200 files (`commands Lock/check/medium`) | the configuration is loaded once and shared by the content comparison, the tag check and the signature check (it was loaded three times); one repository-question memo per check | 1.5 s, 71.4 MB, 274k allocs, 16 git processes | 0.45 s, 45.1 MB, 192k allocs, 10 git processes |
| `lock --check`, 10 files (`Lock/check/small`) | same | 29 ms, 2.19 MB | 9.6 ms, 1.36 MB |
| `lock --check`, 2000 files (`Lock/check/large`) | same | 596 MB, 2.24M allocs | 393 MB, 1.69M allocs |
| `publish --dry-run`, 10 files (`PublishDryRun/bundle/small`) | the load is shared by the lock check, trap files are read at their size instead of into a 1 MiB buffer, the source hash is streamed into the digest, and one memo serves the whole run | 1.49 s, 49.6 MB, 27 git processes | 0.66 s, 37.7 MB, 22 git processes |
| `publish --dry-run`, 200 files (`PublishDryRun/bundle/medium`) | same | 4.3 s, 280 MB, 770k allocs | 2.2 s, 152 MB, 656k allocs |
| `publish --dry-run`, 2000 files (`PublishDryRun/bundle/large`) | same | 2.51 GB, 10.5M allocs | 1.36 GB, 9.89M allocs (-46% bytes) |
| Numbered-path reference check (`lint NumberedPrefixExists`, 100 lookups over 20,000 paths) | the paths are sorted once per tree and each lookup is a binary search, instead of listing and scanning every path per numeric token | 181 ms, 33.2 MB | 0.04 ms, 4.9 KB |
| Generator secret gate (`generator SecretGate/small`, `medium`) | `generate` runs it under a context that remembers `git rev-parse` answers, so the gate asks git once, not twice | 3.5 MB, 13.4k allocs; 3.7 MB, 15.0k allocs; 2 git processes | 1.9 MB, 7.4k allocs; 2.1 MB, 8.6k allocs; 1 git process |
| Process start (`ai-rulez --version`, `GODEBUG=inittrace=1`) | package-level regular expressions of `lint`, `evalimport`, `cmd/commands` and `secretpat` compile on first use (`sync.OnceValue`) instead of at start | 11.7 MB allocated by package init (6.8 MB in ai-rulez packages) | 9.1 MB (4.2 MB) |

What changed where:

- `lock --check` and `publish --dry-run` load once and thread the result. The memo (`gitutil.WithMemo`) is attached to
  the context of one `generate`, `drift`, `lock --check` or `publish` run and dies with it; it is never process-wide,
  and the long-lived callers (the MCP server, the doctor, watch mode) do not get one. Questions about history
  (`HEAD`, refs, file lists) are not memoized.
- The 2.5 GB of `publish --dry-run` at 2000 files was mostly one allocation: the trap check read every candidate
  file into a fresh 1 MiB buffer. The source hash text (built whole, hashed, dropped) and the repeated configuration
  loads of the lock check were the next removable allocations.
- Package-level regular expressions in the four packages are lazy. The `global` archlint rule treats a
  `sync.OnceValue` the way it treats `regexp.MustCompile` (immutable in practice); the three existing
  `OnceValue` globals (in `ard`, `tokens` and `generator/settings`) lose one allowlisted site each.

Behavior that changed deliberately (everything else is byte-identical):

- Frontmatter is split by `internal/frontmatter` in the config loader (`parseFrontmatter`,
  `hasUnclosedFrontmatter`, `FrontmatterProblem`, `IsOKFListing`), in lint (`parseDoc`) and in the generator
  (`frontmatterEnd`, hash injection, `listingFrontmatter`, `ruleFileFrontmatter`, `mappedFrontmatterFields`). Its
  rules now apply to all of them: a UTF-8 BOM before the opening fence is ignored (the loader and the generator used
  to treat the file as having no frontmatter); a fence may carry trailing spaces or tabs, and an indented `---`
  is not a fence (the loader and lint used to trim both sides); the empty block `---`/`---` is a block (the loader
  needed three lines); the blank line after a CRLF block is dropped from the body as it is after an LF block;
  `ruleFileFrontmatter` no longer skips blank lines before the opening fence and no longer takes `---foo` for a
  closing fence; hash injection into a block closed on its first line works. `internal/frontmatter.ClosingLine`
  finds the closing line without building the block, so lint does not copy the YAML.
- Readers now refuse input over their limit through `safefs.ReadLimited` instead of passing a cut one on: an
  attestation file over `MaxBundleBytes` (AR721 at the read, not at the parse), the GitHub token and Sigstore
  service responses over their caps, and the publish inputs (templates, the lock, skill files, dist files and
  tar entries), the signing key, the LLM cache entry and secret. `safefs.ReadFileLimited` opens and reads with the same
  bound.

### Third pass: the process cleanup after each spawn

`internal/runner` ends every run by killing what the command left behind. The guarantee, from the code and the
tests (`TestRunKillsHelpersThatLeaveTheGroup`, `TestRunKillsADetachedSystemBinary`, `TestConcurrentRunsDoNotKillEachOther`):
after `Run` returns, no process of the run is alive. That covers the process group, any descendant seen while the
command ran (a helper that called `setsid` or `setpgid` is still a descendant), and a helper that detached and was
reparented to init between two looks at the table, which is found by the run token every child inherits in its
environment or by the write end of the marker pipe every child inherits as a descriptor (macOS withholds the
environment of platform binaries, so there the descriptor is what finds them). It does not cover a helper that drops
both the variable and the descriptor before detaching, and it must never touch a process of another run. On Windows
the job object gives the guarantee without any table read.

The cost was in the token and descriptor search (`adoptCarriers`): it skips processes older than the command's root
(a descendant cannot be older than its ancestor), but a `Spec.ShortLived` run (every git call) does not read the
process table at start, so it never learned the root's start time and the search covered every process of the user,
one `sysctl` (macOS) or `/proc/<pid>/environ` read (Linux) each, after every spawn.

Now a short-lived run reads the start time of its own root with a single lookup at attach (`processStart`: one
`sysctl kern.proc.pid`, or one `/proc/<pid>/stat`), so the search covers only processes that started since the command,
which is a handful. If that lookup fails the search is unbounded as before, so the guarantee is the same on every
platform; nothing was weakened. The full (non-short) path already had the bound and is unchanged. Windows is unchanged.

Measured on a loaded machine (load average 16 to 28, 850 processes), `go test -bench` of the runner and generator
packages, test binaries built before and after and run back to back. Wall times are dominated by the fork cost under
that load and are not reliable; the allocations and the isolated sweep are.

| Path (benchmark) | Before | After |
| ---------------- | ------ | ----- |
| Sweep alone over the real table of 850 processes (`runner AdoptCarriers`) | 9.5 ms, 1.5 MB, 10k allocs | 0.02 to 0.04 ms, 544 B, 9 allocs |
| One short-lived spawn with cleanup (`runner SpawnCleanup/short`) | 1.85 MB, 7.3k to 7.8k allocs | 0.78 MB, 300 to 420 allocs |
| One full spawn with cleanup (`SpawnCleanup/full`) | unchanged | unchanged |
| Secret gate, 10 files (`generator SecretGate/small`) | 4.2 to 4.5 MB, 18k to 19k allocs | 2.1 MB, 3.7k allocs |
| Secret gate with the memo (`SecretGate/small-memo`) | 2.4 to 2.5 MB, 10k to 11k allocs | 0.9 to 1.2 MB, 0.8k to 2.4k allocs |

The cost of the sweep now depends on how many processes start during the command, not on how many exist.

## What dominates elsewhere

These were measured and not changed here.

- **Process spawns per content file.** Loading a project with 200 skills took 91 s on the loaded machine before the
  branch that batches `git ls-files` and `git check-ignore` per directory (0.5 s with it). The remaining
  wall time of `generate`, `clean`, `lock` and `publish` on 200 to 2000 files is mostly git (`rev-parse`,
  `ls-files`) and file-system calls; their CPU time is a small share.
- **Token counting.** cl100k counting of the prompt text is the largest allocator in a lint run (about a third of
  the bytes, in `regexp2` match objects); the memo that removes it is part of the same separate branch.
- **Served skills.** `lock --check` still builds the served-skill views twice (`mcp.DynamicLockChanges` loads the
  configuration for each view and scans the skills); that code lives in `internal/mcp`.
- **Publish at scale.** What remains of `publish --dry-run` at 2000 files (1.36 GB over 9.9 million objects) is the
  strict lint run (about 60%, mostly cl100k token counting in `regexp2`, which the separate token memo branch
  removes), the lock comparison and the served views.
