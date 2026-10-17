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

The repository-index benchmarks build a synthetic monorepo instead (`internal/testutil.BuildMonorepo`): every scope
holds an `AGENTS.md`, a `.claude/skills/<name>/SKILL.md` and a `.claude/rules/<name>.md`, and the rest of the files
are small sources. The default run covers 100, 1000 and 5000 files over 10, 100 and 500 scopes (a shape needs four
files per scope); `AI_RULEZ_BENCH_LARGE=1` adds 20000 files, which take seconds to write, and moves the
`Resolve`, `Within` and `mcp` query benchmarks to that size.

| Package | Benchmarks |
| ------- | ---------- |
| `internal/parser` | `ParseFrontmatter` (no/small/rich/CRLF/long body), `ParseFrontmatterBatch` |
| `internal/tokens` | `Estimate`, `Count` (cl100k and byte ratio, prose and long-line input) |
| `internal/config` | `LoadConfig`, `LoadSkillResourcesContext` (one installed skill of 1, 100 and 2000 files), `LoadSkillResourcesCatalog` (200 skills) |
| `internal/repoindex` | `Build` (git listing and directory walk), `BuildPreListed`, `ListFiles`, `Resolve`, `Within` on synthetic monorepos |
| `internal/includes` | `LoadWithLocalInclude` |
| `internal/lint` | `LoadTree`, `RunFull`, `Analyzers` (each analyzer alone), `SecretScan` (`DetectSecret`, `RedactSecrets`, `ScanText`), `SecretScanLongLine`, `SecretScanCustomPatterns`, `RunSkillTree`, `AnalyzerSelection`, `NumberedPrefixExists`, `ScannerStage` (stage listing, digest and cache key of an external scanner over 50 and 500 skills) |
| `internal/contentlock` | `Compute`, `BuildAndCompare` (lock digest, then verify) |
| `internal/lockfile` | `SaveLoad` |
| `internal/generator` | `Generate` (full and idempotent no-op), `GeneratePresets` (2, 10 and every preset), `GenerateLLMsTxt`, `Clean` (remove and dry run), `CheckDrift`, `SecretGate` (plain and `-memo`, the context `generate` runs under) |
| `internal/review` | `Score` (collect and score), `Calibrate` |
| `internal/mcp` | `ProjectTools` (tool dispatch), `SkillServer` (catalog build, `search_skills`, `load_skill`), `DiscoveryCatalog` (catalog build with repository discovery on a monorepo), `RepositoryQueries` (`find_skill` and `find_context` with a path or scope) |
| `internal/verifiers` | `RegexFiles` |
| `cmd/commands` | `Lock` (write and check), `PublishDryRun` (bundle, agent-plugins, ard), `OKF` (export, validate) |

## Regression tracking

CI does not compare against a number recorded on another machine. The `Benchmarks` job checks out the full history
and runs `scripts/bench.sh --compare`, which measures the base revision and `HEAD` on the same runner, in the same job:

1. It picks the base: the merge base with the default branch, or the parent commit when `HEAD` is on it (`--compare REF`
   or `BENCH_BASE` overrides it).
2. It builds the test binary of each package of a curated subset at both revisions (benchmarks that start no process
   and do not fsync, so they measure the code and not the machine; a package that does not build at the base is skipped
   with a notice).
3. It takes `BENCH_COUNT` samples (6) of each benchmark per revision, interleaved base, head, head, base, ..., so a
   machine that warms up or is throttled favors neither.
4. It compares the medians and runs a two-sided Mann-Whitney U test on the samples, the test `benchstat` uses.

A benchmark fails the job only when it is both more than `BENCH_THRESHOLD` percent slower (20) and different at
`BENCH_ALPHA` (0.05). A slowdown that is not significant is printed as `slower, not significant`, and a run with
fewer than 5 samples is refused, because it cannot reach the significance level. The large synthetic sizes stay out of
the subset; they run only with `AI_RULEZ_BENCH_LARGE=1`. The rest of the benchmarks run once in the job
(`task test:benchmark:smoke`, `-benchtime=1x`) to prove they still run.

```bash
task test:benchmark:regression                    # HEAD against the merge base, on this machine
scripts/bench.sh --compare origin/main            # the same, with an explicit base
BENCH_THRESHOLD=10 BENCH_COUNT=10 scripts/bench.sh --compare HEAD~3
```

When a run flags a benchmark you believe is noise, run it again locally with more samples
(`BENCH_COUNT=12`); a real regression stays significant. The job runs with `shell: bash` (so `pipefail` holds), and
uploads `bench-subset.txt`, the head samples. A change that intentionally makes a benchmark slower is not overridden by
a file: say so in the pull request and rerun the job; a maintainer can merge over a failure that the table explains.

`scripts/bench-baseline.txt` and `scripts/bench.sh` without an argument remain as a fallback for a developer machine:
the script compares `HEAD` with the committed file, but only when it was recorded on the same CPU model (the `cpu:` line
of the output), and otherwise warns and exits 0. Refresh it with `scripts/bench.sh --update` (`task
test:benchmark:baseline`). CI does not read it.

## Monorepo index

`internal/repoindex` indexes the skills, instruction files, rules, agents and commands a repository keeps at any
depth (`mcp --serve-skills --discover-skills`). The numbers below are a cold build: the file listing, the
classification of every path, one read per item for its description and hash, and the grouping into scopes. They
come from `go test -bench` on the synthetic monorepo described above, 5 iterations per row, on a 4-vCPU Intel Xeon
2.1 GHz virtual machine with a slow file system for creating files. Compare rows with each other, not with
your machine or with the CI runners.

Cold `Build`, in ms (B/op in MB), by files and scopes. `git` lists files with `git ls-files`; `walk` is the fallback
in a directory that is not a repository.

| Files | Scopes | Items | git | git B/op | walk | walk B/op |
| ----- | ------ | ----- | --- | -------- | ---- | --------- |
| 100 | 10 | 30 | 11.1 | 1.3 | 5.9 | 0.9 |
| 1000 | 10 | 30 | 14.7 | 1.8 | 9.8 | 1.4 |
| 1000 | 100 | 300 | 25.0 | 5.3 | 22.8 | 5.3 |
| 5000 | 10 | 30 | 19.1 | 3.7 | 15.2 | 3.0 |
| 5000 | 100 | 300 | 47.4 | 7.2 | 51.3 | 8.5 |
| 5000 | 500 | 1500 | 78.2 | 23.3 | 89.7 | 25.0 |
| 20000 | 10 | 30 | 34.1 | 17.4 | 41.7 | 9.0 |
| 20000 | 100 | 300 | 51.4 | 23.7 | 88.5 | 14.8 |
| 20000 | 500 | 1500 | 129.1 | 49.8 | 202.2 | 37.9 |

The cost has two parts. The listing grows with the files: 36 ms for `git ls-files` on 20000 files and 67 ms for the
walk (`ListFiles`). Reading the items grows with their number, about 20-35 us each (a stat, a read, a YAML parse of
the frontmatter, a SHA-256): with the listing supplied (`BuildPreListed`) 20000 files take 4.1 ms for 30 items, 13.3 ms
for 300 and 35.7 ms for 1500. Classification costs about 0.2 us per file and is not a factor.

Queries on the built index are microseconds:

| Operation (20000 files) | ns/op | B/op | allocs/op |
| ----------------------- | ----- | ---- | --------- |
| `Resolve` of a path six directories deep, 10 / 100 / 500 scopes | 2400 / 2300 / 2300 | 2128 | 8 |
| `Within` a leaf scope | 9600 | 968 | 6 |
| `Within` the root, recursive (1500 items) | 371000 | 614344 | 16 |

`Resolve` walks the ancestors of the path and looks each up in a map, so it does not depend on the number of scopes.

Through the MCP server, the catalog build with discovery on (`BenchmarkDiscoveryCatalog`, 20000 files) is the sum of
the index build, reading every served skill directory, and the security scan of every item (`lint.ScanServed` for
skills, the index `Screen` for instruction files, rules and agents):

| Scopes | Items | Catalog build, median of 5 | B/op | allocs/op |
| ------ | ----- | -------------------------- | ---- | --------- |
| 10 | 30 | 65 ms | 22.1 MB | 130k |
| 100 | 300 | 291 ms | 58.8 MB | 363k |
| 500 | 1500 | 1220 ms | 220.8 MB | 1.30M |

The index cache (`[mcp] index_cache`, `docs/mcp-server.md#the-index-cache`) removes most of that on a later start.
`BenchmarkDiscoveryCatalogCache` starts the same server three ways on the 20000-file monorepo: `off` keeps no cache,
`cold` starts with an empty cache and writes it, `warm` starts with the cache of an earlier start, over files whose
modification time is an hour old (a file modified in the two seconds before a save is read again). Mean of 5 starts,
same machine as above:

| Scopes | Items | off | cold (fills the cache) | warm | warm B/op | warm vs off |
| ------ | ----- | --- | ---------------------- | ---- | --------- | ----------- |
| 10 | 30 | 48.9 ms | 58.1 ms | 22.7 ms | 5.5 MB | -54% |
| 100 | 300 | 316.8 ms | 292.4 ms | 53.5 ms | 11.3 MB | -83% |
| 500 | 1500 | 1251 ms | 1450 ms | 106 ms | 37.3 MB | -92% |

A cold start with the cache on costs what a start without it does, plus the write of the index file (the 10 and 500 scope
rows differ from `off` by run-to-run noise as much as by that write). A warm start no longer reads, hashes or scans an
unchanged item: what remains is the file listing, one `stat` per candidate, the configuration load and reading the
served skill directories. The live refresh of the index pays the warm cost per change, not the cold one.

Most of that is the security scan, not the index (before the fifth pass about 90% of a build; `Build` alone is 129 ms at 500 scopes). Queries over the
in-memory transport (`BenchmarkRepositoryQueries`, one sample each; every call also pays about 0.5 MB and 0.3 ms of
protocol overhead, the same as `search_skills`):

| Query | 10 scopes | 100 scopes | 500 scopes |
| ----- | --------- | ---------- | ---------- |
| `find_skill` with a task | 0.59 ms | 0.76 ms | 1.71 ms |
| `find_skill` with a task and a path | 0.51 ms | 0.71 ms | 1.73 ms |
| `find_skill` with a scope, recursive | 0.38 ms | 0.67 ms | 1.52 ms |
| `find_context` with a path | 0.51 ms | 0.54 ms | 0.46 ms |
| `find_context` with a task and a path | 0.33 ms | 0.34 ms | 0.32 ms |
| `find_context` with a scope, recursive | 0.76 ms | 0.82 ms | 1.09 ms |

`find_skill` ranks the whole served catalog, so it grows with the skills (500 at most, the discovery cap);
`find_context` only ranks the scopes that apply to the path.

`TestBuildOfAMonorepoStaysFast` fails when a cold build of 5000 files over 100 scopes takes more than 2 seconds (it
measures 50-70 ms); it is skipped under `-short` and the race detector.

What a cache would save: the index build itself is 5-130 ms even at 20000 files, so caching only the listing and
the item summaries (the proposal's phase 3) removes at most that. The larger cost in the server is the per-item
security scan, which a cache keyed by content hash would also skip.

## Installed skill bundles

Installing or serving a skill reads its whole directory (`config.LoadSkillResourcesContext`): every file besides
`SKILL.md` that the bundle excludes and the `.gitignore` of the work tree allow, with a description taken from the
frontmatter of each Markdown file. Same machine and method as above.

| Bundle | ms/op | B/op | allocs/op |
| ------ | ----- | ---- | --------- |
| One skill, 1 file | 2.9 | 428 KB | 1466 |
| One skill, 100 files | 5.7 | 1.7 MB | 9303 |
| One skill, 2000 files | 52.3 | 26.1 MB | 157556 |
| 200 skills of 10 files each | 839 | 119 MB | 538587 |

A bundle has a fixed cost of about 3 ms (the two git processes that ask which files .gitignore leaves out, one since
the fifth pass) and then 25 us per file. The 2000-file bundle is
at the file limit of an installed skill.

The external scanner stage (`ScannerStage`) is what a `[[lint.external]]` scanner needs before it starts anything:
listing the staged files, digesting them and computing the cache key. The key itself is 4-5 us whatever the size.

| Skills (4 files each) | Stage listing | Digest | Cache key |
| --------------------- | ------------- | ------ | --------- |
| 50 | 1.45 ms, 612 KB, 3821 allocs | 0.25 ms, 17 KB | 5.0 us |
| 500 | 14.4 ms, 6.2 MB, 38043 allocs | 2.69 ms, 164 KB | 4.1 us |

Both grow linearly. A scanner run on a warm cache therefore costs the stage listing plus the digest, 1.7 ms for 50
skills and 17 ms for 500.

`generate` end to end on the 200-file benchmark project (`GeneratePresets`, median of 3), with 2, 10 and all 55
built-in presets:

| Presets | Full generate | B/op | No-op generate | B/op |
| ------- | ------------- | ---- | -------------- | ---- |
| 2 | 243 ms | 19.9 MB | 142 ms | 26.6 MB |
| 10 | 500 ms | 61.6 MB | 213 ms | 73.0 MB |
| 55 | 1882 ms | 269 MB | 670 ms | 309 MB |

The time is spent writing the outputs (a temporary file and a rename per output) and rendering, about 30 ms per
preset; nothing in it was changed.

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

### Fourth pass: CPU of `generate` and `validate`

v5.0.0 spent about four times the CPU of 4.23 in `generate` (and more in `validate`). A CPU profile (a temporary
`pprof` hook, not committed) put about half of `generate` in the authored-content secret gate, which called
`lint.RunWith` with `SecurityOnly` although it reads only AR001: every file went through the shell, injection,
hidden-character, host and blob scans, and the markdown file was split into lines twice (`newScanText`, dominated by
backtracking in `negRe`). `validate` paid the same duplicate splits plus unconditional regular expressions in the body
scan.

Measured on a 590-file, 2.6 MB project, user+sys CPU of the whole command, alternating runs of binaries built before
and after (load average high; sys time is file I/O and did not change):

| Command | user before | user after | user+sys before | user+sys after |
| ------- | ----------- | ---------- | --------------- | -------------- |
| `generate` | 1.40 s | 0.65 s | 3.3 to 3.6 s | 2.3 to 2.4 s |
| `validate` | 1.9 s | 1.6 s | 3.6 s | 3.2 to 3.3 s |
| `tokens` | 0.69 s | 0.69 s | 1.9 s | 1.9 s (unchanged) |

Secret gate alone (`lint AuthoredSecretGate/medium`, 200 files): 106 to 161 ms, 13.0 MB, 115.6k allocs with the full
security family; 33 ms, 2.9 MB, 20.4k allocs with `Options.SecretsOnly` (3 to 5x).

- `Options.SecretsOnly` (with `SecurityOnly`) runs only the units that reach `securityScan` (item scan, resources,
  imported content) and checks each line against the credential patterns alone. Inline ignores, severities,
  `ignore`, `example_paths`, configured patterns and the imported-content level go through the same `add`, so the AR001
  findings are identical (`TestSecretsOnlyMatchesSecurityOnly`).
- `scanTextOf` memoizes the split text per file and content for the run (security scan, text scans, capability
  profile); a `scanText` is never modified after it is built (`TestScanTextOfMemoizesPerFileAndText`).
- The body scan skips link, backtick and name patterns on lines lacking a literal every match contains
  (`TestBodyScanGatesAgreeWithThePatterns` compares gated and ungated results). `generate` output and
  `validate`/`validate --strict`/`tokens` output on the benchmark project are byte-identical to the previous binary.

`tokens` is dominated by cl100k counting and file reads, and was not changed (the token memo is the separate branch
named below). What remains of `generate` is file I/O (loading, output safety checks, writes).

### Fifth pass: the security scan and the git questions

Test binaries built from the commit before the pass and from its last commit, run alternately three times on the
machine described under Monorepo index; medians. The `Benchmarks` job of CI failed on main because
`SecretScan/ScanText` was 72-93% over `scripts/bench-baseline.txt` (the 25% gate): the rules
`credential-store-instruction` and `permission-bypass-instruction` had been added since the baseline and ran their
regular expressions on every prose line. They now run behind literal checks, like the other text scans.

| Path (benchmark) | Change | Before | After |
| ---------------- | ------ | ------ | ----- |
| Security scan of one text, 37 KB (`lint SecretScan/ScanText/clean/large`) | credential-store scan runs its regular expressions only on a line holding a store path or a word a store is named by; each permission-bypass pattern runs only on a line holding its rarest word (not "auto", which "author" holds); the case-folded stem search jumps between candidate letters | 20.1 ms | 10.9 ms (-46%) |
| Same, 3 KB (`ScanText/clean/small`) | same | 1.20 ms | 0.66 ms (-45%) |
| Same, text with secrets (`ScanText/dirty/large`) | same | 20.1 ms | 10.7 ms (-47%) |
| Security analyzer of a 200-file project (`lint Analyzers/security`) | same | 74.3 ms | 42.7 ms (-43%) |
| Lint of 200 content files (`RunFull/medium`) | same | 146 ms | 112 ms (-24%) |
| Catalog build with discovery, 20000 files, 10 / 100 / 500 scopes (`mcp DiscoveryCatalog`) | same (the scan of every served item) | 82 / 441 / 2246 ms | 62 / 306 / 1239 ms (-24% / -31% / -45%) |
| One installed skill of 1 file (`config LoadSkillResourcesContext/files=1`) | the bundle filter skips `git ls-files` when `git check-ignore` already says the directory is no work tree (two processes became one) | 5.9 ms, 828 KB, 2690 allocs | 2.9 ms, 428 KB, 1466 allocs (-51%) |
| 200 installed skills of 10 files (`LoadSkillResourcesCatalog`) | same | 1386 ms, 199 MB | 839 ms, 119 MB (-39%) |

Output is unchanged: `TestMayNameCredentialKeepsEveryStore`, `TestBypassStemsKeepEveryPattern` and
`TestIndexFoldASCIIAgreesWithTheReference` compare each gate with the pattern it guards, and the existing lint
tests pass. Two benchmarks also failed outright and were fixed: `BenchmarkProjectTools` called tools with a
`working_directory` outside the server's confinement, and `BenchmarkSkillServer/load_skill` hit the 256 KiB session
budget once `N` passed about 2700. CI did not report it, because `task test:benchmark | tee` returned tee's exit status;
the step now runs with `pipefail`. `scripts/bench-baseline.txt` still holds numbers from before the new rules and
should be re-recorded from the CI artifact (`bench-subset.txt`).

Measured and left alone: the 10.9 ms that remains in `ScanText/clean/large` is spread over about thirty rules, none
above 10%; the index build proper (listing, classification, reads) is under 10% of a catalog build, so its per-item cost (20-35 us, mostly the YAML parse and
the read) was not worth changing.

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
