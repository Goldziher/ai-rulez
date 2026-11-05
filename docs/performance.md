# Performance

What the common ai-rulez operations cost on a typical project, which settings change that, and how a slowdown in
ai-rulez itself is caught before release.

The numbers below come from the benchmarks in the repository on synthetic projects, measured on a 4-vCPU Intel Xeon
2.1 GHz virtual machine with a slow file system, so rows dominated by file writes (such as `generate`) are
likely faster on a local SSD. Compare rows with each other; your machine will differ in absolute terms.

## At a glance

| Operation | Project | Time |
| --- | --- | --- |
| `generate`, 2 presets | 200 content files | 243 ms (142 ms when nothing changed) |
| `generate`, 10 presets | 200 content files | 500 ms (213 ms when nothing changed) |
| `generate`, every built-in preset | 200 content files | 1.9 s (670 ms when nothing changed) |
| Strict lint of the content (`validate`) | 200 content files | 112 ms |
| Loading one installed skill | 1 / 100 / 2,000 files | 3 ms / 6 ms / 52 ms |
| Loading 200 installed skills | 10 files each | 0.84 s |
| MCP server start with skill discovery, cold | 20,000-file monorepo, 30 / 300 / 1,500 items | 65 ms / 291 ms / 1.2 s |
| Same, with a warm index cache | same | 23 ms / 54 ms / 106 ms |
| `find_skill` / `find_context` query | same | under 2 ms |

## generate

`generate` spends its time rendering and writing outputs, about 30 ms per preset on the 200-file project. A run in
which nothing changed costs roughly a third to a half as much.

- List only the presets your team uses in `presets`; each one adds rendering and writes.
- Use `generate --check` in CI to compare the committed outputs with the sources without writing.

## validate and lint

The strict lint of a 200-file project takes about a tenth of a second. On large projects, report only what changed:

```bash
ai-rulez validate --changed            # against HEAD
ai-rulez validate --changed=origin/main
```

An external scanner (`[[lint.external]]`) is cached by a digest of the files it scans. With a warm cache, preparing
the scan costs about 1.7 ms for 50 skills and 17 ms for 500, and grows linearly.

## Installed skills

Installing or serving a skill reads its whole directory, honoring the bundle's excludes and the work tree's
`.gitignore`. That costs about 3 ms per skill plus 25 microseconds per file. A skill at the 2,000-file limit takes
about 50 ms; a catalog of 200 installed skills of 10 files each takes under a second.

## MCP server with skill discovery

With `mcp --serve-skills --discover-skills` (or `[mcp] discover_skills`), the server indexes the skills, instruction
files, rules, agents and commands of the whole repository at start-up and security-scans every item before serving it.

Cold start (no index cache), 20,000 files:

| Scopes | Items | Catalog build | Memory allocated |
| --- | --- | --- | --- |
| 10 | 30 | 65 ms | 22 MB |
| 100 | 300 | 291 ms | 59 MB |
| 500 | 1,500 | 1.2 s | 221 MB |

The security scan is most of that. The [index cache](mcp-server.md#the-index-cache) (`[mcp] index_cache`, on by
default with `"user"`) stores summaries and scan verdicts, so a later start rereads only changed items:

| Scopes | Items | No cache | Cold (fills the cache) | Warm | Warm vs no cache |
| --- | --- | --- | --- | --- | --- |
| 10 | 30 | 49 ms | 58 ms | 23 ms | -54% |
| 100 | 300 | 317 ms | 292 ms | 54 ms | -83% |
| 500 | 1,500 | 1.25 s | 1.45 s | 106 ms | -92% |

Keep the cache on. Setting `index_cache = "off"` makes every start pay the cold cost, and the live refresh of the
index pays the warm cost per change.

Queries on a running server, 20,000 files:

| Query | 10 scopes | 100 scopes | 500 scopes |
| --- | --- | --- | --- |
| `find_skill` with a task | 0.59 ms | 0.76 ms | 1.71 ms |
| `find_skill` with a task and a path | 0.51 ms | 0.71 ms | 1.73 ms |
| `find_context` with a path | 0.51 ms | 0.54 ms | 0.46 ms |
| `find_context` with a task and a path | 0.33 ms | 0.34 ms | 0.32 ms |

`find_skill` ranks the whole served catalog (at most 500 discovered skills), so it grows with the number of skills;
`find_context` ranks only the scopes that apply to the path.

### Repository index

The index build alone (without the security scan) is fast even on large monorepos. Cold build in milliseconds, by files
and scopes; `git` lists files with `git ls-files`, `walk` is the fallback outside a git repository:

| Files | Scopes | Items | git | walk |
| --- | --- | --- | --- | --- |
| 1,000 | 100 | 300 | 25 | 23 |
| 5,000 | 100 | 300 | 47 | 51 |
| 5,000 | 500 | 1,500 | 78 | 90 |
| 20,000 | 100 | 300 | 51 | 89 |
| 20,000 | 500 | 1,500 | 129 | 202 |

Listing grows with the number of files (about 36 ms for `git ls-files` on 20,000 files); reading items costs about 20
to 35 microseconds each.

## Model-backed commands

`judge`, `review --semantic`, `eval run` and `improve` are bound by the model provider, not by ai-rulez. Their cost and
duration are dominated by the number of model calls:

- Run the estimate first: `judge --estimate`, `review --estimate`, `eval run --dry-run`, `improve run --dry-run`.
- Use `--changed` (`judge`), `--since` (`review`) or `--changed-only` (`eval run`) to limit the run to what changed.
- Responses are cached per user ([LLM access](llm.md)); an unchanged project costs nothing on a re-run.
- `judge --concurrency` (default 4) and `--rpm` trade speed against provider rate limits.

## Running the benchmarks

```bash
mise run test:benchmark          # every benchmark, small and medium synthetic projects
mise run test:benchmark:large    # adds the 2,000-file project and 20,000-file monorepos
go test ./internal/lint -run '^$' -bench RunFull -benchmem
```

Benchmarks build synthetic projects in a temporary directory and make no network calls.

## Regression tracking

CI compares benchmarks against each change's base revision; see the [maintainer notes](maintainers/benchmarks.md).
