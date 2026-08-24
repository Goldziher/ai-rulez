# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project adheres to Semantic Versioning.

## [4.11.3] - 2026-08-24

### Fixed

- `generate` is now reproducible across checkout paths. `computeSourceHash` folded the raw absolute `ContentFile.Path` into the source hash, so the same tree generated from two directories produced different `Source-Hash` values. The bodies were byte-identical, but the mismatch forced a rewrite and stamped a fresh `Generated:` timestamp, leaving clean-checkout CI drift checks permanently dirty. Paths are now normalized before hashing — relative to the config or base directory when in-tree, collapsed to their last two segments when not. The fallback also removes three cases the report did not cover: git includes cached under the user's home directory (so a laptop and a CI runner disagreed at the same commit), installed skills, and the randomly-named temp symlink used for bare include layouts (non-deterministic run to run on one machine). Normalizing through `filepath.ToSlash` additionally stops Windows and Linux disagreeing about the same tree. `GeneratorSchemaVersion` moves to `v3`, so every project regenerates once before the skip mechanism re-engages. (#166)
- `ai-rulez mcp` no longer uses the deprecated `ServerOptions.HasTools`; tools are advertised through `Capabilities` with the same `{"listChanged":true}` value. Setting `Capabilities` suppresses the SDK's default `logging` capability, which is deprecated as of protocol version 2026-07-28 and which this server never emitted. This also unblocks the Lint job, which staticcheck's SA1019 had been failing on `main` since the SDK bump in 4.11.2.

### Added

- `[header] timestamp = false` omits the `Generated:` line from all three header styles, for projects that commit their generated outputs and want no per-run value in the header at all. Defaults to `true`, so existing output is unchanged.

### Changed

- Dependencies updated: `samber/oops` 1.23.1, `golang.org/x/text` 0.41.0, with indirect bumps to `golang.org/x/net` 0.58.0, `golang.org/x/crypto` 0.55.0, and `go-json-experiment/json`; docs toolchain to zensical 0.0.57. `govulncheck` reports no known vulnerabilities.
- Workflow actions updated: `golangci-lint-action` v7 → v9, `xberg-io/actions` reusable-validate v1.8.142 → v1.8.145, and `astral-sh/setup-uv` v6 → v10.0.1. setup-uv is pinned to a full semver tag because it stopped publishing major and minor tags at v8 as a supply-chain measure, so `@v10` does not resolve.
- The JSON schema's `header.style` default now reads `minimal`, matching the code default since 4.9.

## [4.11.2] - 2026-08-08

### Fixed

- `ai-rulez mcp` no longer wedges when a host sends `initialize` twice on one stdio session. The MCP SDK treats initialization as a one-shot state machine, so a repeated `initialize` failed with `duplicate "initialize" received` (and a repeated `notifications/initialized` likewise), permanently breaking any host that retries or reconnects over a long-lived server process — Claude Code's MCP client, or an mcpm/fastmcp bridge shared between consumers. A repeated `initialize` is now answered with the result of the original negotiation and a repeated `notifications/initialized` is dropped, leaving the session intact. Because the SDK's version negotiation is internal, a re-initialize receives the protocol version agreed on first connect. (#158)

### Changed

- Dependencies updated: `kaptinlin/jsonschema` 0.9.8, `oklog/ulid` 2.1.2, OpenTelemetry 1.45.0, `go.yaml.in/yaml` 3.0.5; docs toolchain to zensical 0.0.53. The unused `tool github.com/evilmartians/lefthook` directive was dropped, removing 27 indirect modules from `go.mod` — the repo moved off lefthook to poly hooks and nothing imported it.
- `task update` now updates the whole Go module graph plus `uv.lock`, and `task lint` runs the poly checks. Both previously invoked `prek` against a `.pre-commit-config.yaml` that no longer exists.

## [4.11.1] - 2026-07-31

### Fixed

- `generate` no longer inlines the full rules block into every generated `.claude/skills/<name>/SKILL.md` and `.claude/agents/<name>.md`. A rule targeting the `claude` preset name was matching any output under `.claude/`, so it was duplicated into every per-item skill/agent file; it now routes to `CLAUDE.md` only. Explicit path, directory, and glob targets are unaffected. (#156)
- `generate` no longer emits a second, raw frontmatter block in a skill file when the source frontmatter fails to parse. Malformed YAML frontmatter (e.g. an unquoted value containing `": "`) was returned unstripped and re-emitted after the generated block; it is now stripped with a warning. The 14 builtin skills whose `description` contained an unquoted `": "` are quoted so their descriptions parse and populate the generated frontmatter. (#156)

## [4.11.0] - 2026-07-22

### Added

- `ai-rulez clean` command: removes the files produced by `generate` (the inverse of `generate`) — the generated assistant outputs (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `.claude/`, `.codex/`, generated skills, …), the generated manifest, and the ai-rulez managed `.gitignore` block. The `.ai-rulez/` source tree is never touched and generated directories are removed only once empty (files you authored inside them are kept). Lists targets and prompts for confirmation by default; `--dry-run` previews, `--force` skips the prompt, `--keep-gitignore` / `--keep-manifest` preserve those. Exposed over MCP as `clean_outputs`.

### Fixed

- MCP `update_rule` / `update_context` / `update_skill` no longer fail with `file already exists` when updating existing content. The update path used a create-only write primitive that refused to overwrite; it now uses an overwrite-capable atomic write. (#150)

## [4.10.0] - 2026-07-22

### Added

- Local-override content: drop machine-local rules and context under `.ai-rulez/local/rules/` and `.ai-rulez/local/context/` and they are emitted only to per-preset `.local` root files (`CLAUDE.local.md`, `AGENTS.local.md`, `GEMINI.local.md`, `.junie/guidelines.local.md`, `.github/copilot-instructions.local.md`, …). Local content is kept strictly separate from committed output and both the `.local` files and the `.ai-rulez/local/` source directory are gitignored unconditionally (even when `gitignore = false`). New `--local` flag on `ai-rulez add rule` and `ai-rulez add context` writes there directly.
- Bare/flattened include layout: included repositories no longer need to wrap their content in an `.ai-rulez/` directory; a flattened `rules/`, `context/`, `skills/` layout is now supported.

### Changed

- Built-in language, binding, OWASP, and dependency-awareness conventions now emit as on-demand Agent Skills (`skills/<name>/SKILL.md`) instead of always-inlined rules/context, shrinking the generated `CLAUDE.md` (and peers) considerably. The agent loads them only when the relevant "Load when…" trigger applies.
- Default header style is now `minimal` (was `detailed`), trimming ~37 lines of boilerplate from every generated root file while keeping the DO-NOT-EDIT warning plus the Content-Hash / Source-Hash provenance lines. `detailed` and `compact` remain available via `[header] style = "…"`.

## [4.9.4] - 2026-07-12

### Changed

- Built-in language and binding convention rules are now tool-agnostic. They describe idiomatic principles and quality bars rather than mandating one third-party stack: opinionated tools (e.g. `mypy`, `oxlint`, `oxfmt`, `structlog`, `vitest`) are now framed as examples, while canonical/official toolchains (`gofmt`, `cargo fmt`, `dotnet format`, `tsc`, `mix format`, …) are retained.

### Fixed

- Python builtin convention no longer references `Unknown` (a TypeScript type); it now recommends precise types, generics, or `typing.Protocol`.
- Fixed typos in the vite+ builtin convention.


## [4.9.3] - 2026-07-12

### Fixed

- Reject Windows drive-relative plugin paths such as `C:outside` on every host platform.

### Changed

- Pin Poly catalog npx and uvx execution paths to the catalog release for reproducible hook runs.

## [4.9.2] - 2026-07-12

### Fixed

- Validate plugin paths consistently across operating systems and use portable path assertions for Hermes and OpenCode output tests.

### Changed

- Split plugin authoring validation into focused checks to keep complexity within project limits.

## [4.9.1] - 2026-07-12

### Fixed

- Use the ai-rulez `version` subcommand in Poly hook installation paths.

## [4.9.0] - 2026-07-12

### Added

- Reusable `poly-hooks.toml` catalog with multiple selectable hooks, guarded npx, uvx, and system execution paths, explicit managed install commands, and parity with the pre-commit hook catalog.
- Recursive plugin generation and verification with `--if-configured`, including atomic marketplace traversal without duplicate member work.
- Plugin generation and verification hooks for both Poly and pre-commit, triggered by root or nested `.ai-rulez/` changes.

### Changed

- Poly consumers declare local or Git sources in `poly.toml` and can select non-mutating validation and plugin verification while keeping generation and auto-fix hooks opt-in.

## [4.8.0] - 2026-07-12

### Added

- Native Hermes Agent rules generation through the `hermes` preset and `.hermes.md` project context.
- Hermes Agent project plugins and PyPI entry-point packages, with compatible `hermes`, `register`, and `__version__` exports.
- Plugin-specific content through `plugin.content_root`, adapter reuse through `plugin.hermes.source`, and configurable Python compatibility through `plugin.hermes.requires_python`.
- Generated plugin freshness and provenance verification with `ai-rulez verify --plugin`, including profile-aware rendering.

### Fixed

- Preserve authored Markdown whitespace when inserting and verifying generated-file provenance headers.
- Serialize Hermes Python package metadata safely and emit Claude marketplace files only when targeting Claude.

## [4.7.0] - 2026-07-11

### Added

- Complete Codex plugin bundles with root MCP configuration, canonical marketplace metadata, recursive skill resources, and validated interface assets.
- OpenCode plugin generation from `.ai-rulez/opencode/index.js`, including generated package metadata and a documented no-op scaffold when no adapter is authored.
- Deterministic plugin provenance through generated-file headers and `.ai-rulez-generated.json` sidecars with BLAKE3 content and source hashes.

### Fixed

- Preserve nested skill resources and Codex interface metadata during plugin generation.
- Emit runtime-safe relative MCP commands and strict JSON manifests without comment headers.

## [4.6.0] - 2026-07-08

### Added

- **Plugin & marketplace authoring** via `ai-rulez generate --plugin`: packages the project's skills, commands, agents, and MCP servers into distributable plugin bundles and a marketplace index for Claude, Cursor, Codex, Gemini, Kimi, OpenCode, and Factory. A new `[plugin]` config block carries the packaging metadata (kept distinct from the consumer `[[plugins]]`/`[[marketplaces]]` install arrays), with hooks, a Claude status-line passthrough, a canonical `${PLUGIN_ROOT}` launch variable rewritten per runtime, and both single-plugin and monorepo (`[marketplace].members`) marketplaces.
- Agent **`extends` directive**: an agent whose frontmatter sets `extends: <name>` inherits the body and frontmatter of a lower-precedence agent (the same name in a lower layer, or the named target) and appends its own body, with set frontmatter fields overriding the base and omitted fields inherited. Chains resolve across multiple layers (local extends include extends builtin); a missing base or an `extends` cycle degrades to a plain agent with the directive stripped.

### Fixed

- Agent precedence is now deterministic and matches rules/context: same-named agents collapse to the single highest-precedence definition (local > include > builtin) instead of a non-deterministic last-write-wins that depended on domain name ordering. The generated header agent count also reflects the deduplicated set.

### Changed

- Built-in agent default models: `docs-writer`, `devops-engineer`, and `release-engineer` now use `sonnet` (was `haiku`); `polyglot-architect` now uses `opus`.

## [4.5.0] - 2026-07-02

### Added

- Rule and context **deduplication by name** with source precedence (root > on-disk domain > include > builtin). When the same name is defined by more than one source (for example a builtin `git-workflow` rule and an include redefining `commit-messages`), the generated output now includes it only once, and a warning is logged during `generate` and `validate` naming the kept and dropped sources.
- `compact` config option: when `true`, generated inline rule sections omit the per-rule `**Priority:**` annotations to reduce output size.
- Per-rule builtin exclusion via the `!domain/rule` syntax in the `builtins` array (e.g. `!git-workflow/commit-messages`), which drops a single builtin rule while keeping the rest of the domain.
- MCP tool annotations for all write tools (create/add/update/set marked additive or idempotent rather than defaulting to destructive), plus server `Title` and initialization `Instructions` metadata.

### Fixed

- MCP server configuration for remote (`http`/`sse`) transports no longer emits an empty `command` or a `transport` key. Each preset now emits its tool-specific remote format: Claude `type` (#136, #137), Gemini `httpUrl`/`url`, Copilot `type`, Cursor `url`, and Antigravity `serverUrl`. Stdio servers are unchanged.

### Changed

- Migrated preset rendering to a declarative DSL provider system (Claude, Amp, Junie, and MCP specs), replacing the hand-written generators for those presets.
- Migrated lint and format tooling from prek to poly.
- Updated Go dependencies (`pelletier/go-toml/v2` 2.4.2, `kaptinlin/jsonschema` 0.9.2, `golang.org/x/text` 0.38, `golang.org/x/net` 0.56) and CI actions (`actions/checkout` v7, `actions/cache` v6).

## [4.4.1] - 2026-06-07

### Fixed

- `.ai-rulez/.generated-manifest.json` is now included in the managed `.gitignore` fence when `--gitignore` (or `gitignore: true` in config) is enabled. The manifest is rewritten on every `generate`; tracking it produced endless diff noise. Honours custom `config_dir` values.

### Tests

- Added end-to-end coverage for per-preset model overrides so the resolver chain (agent `<preset>_model` → `defaults.model_by_preset` → legacy `model:`) is exercised against real generated agent files for both Claude and Copilot.

## [4.4.0] - 2026-06-07

### Added

- Per-preset model overrides for agents. Each preset now resolves its agent `model` value via `<preset>_model` frontmatter (e.g. `claude_model`, `copilot_model`, `cursor_model`) with `defaults.model_by_preset` as a project-wide fallback. The legacy single `model:` field remains supported as the lowest-priority fallback for backward compatibility.

### Fixed

- The TOML config loader silently dropped the entire `[defaults]` table, so `effort_by_preset` from TOML configs never reached the generator. The new `model_by_preset` feature exposed the gap; both tables now load correctly.

### Changed

- Updated Go dependencies (`agentable/go-intl`, `go-json-experiment/json`, `kaptinlin/go-i18n`, `kaptinlin/jsonpointer`, `kaptinlin/jsonschema`, `kaptinlin/messageformat-go`) and pre-commit hooks (`kreuzberg-dev/pre-commit-hooks` 1.2.3 → 2.1.8, `gh-actions-updater` 0.1.5 → 0.1.6, `Goldziher/ai-rulez` self-reference 4.3.1 → 4.3.2). Go toolchain bumped from 1.26.3 to 1.26.4.

## [4.3.0] - 2026-05-30

### Added

- Added `generate --gitignore` with `-i` shorthand; the old `--update-gitignore` flag remains as a deprecated compatibility alias.
- Added collision-safe short flags across global, generate, CRUD, list, domain, profile, include, init, validate, and skill commands.
- Added MCP environment placeholder resolution from repeated `--env KEY=VALUE`, process environment, `.env`, and explicit `--env-file` sources.

### Changed

- `.gitignore` generation now writes generated roots and directory patterns instead of per-file assistant output entries, while keeping generated `.github/` paths scoped to Copilot-owned files and directories.

### Fixed

- Secret-bearing MCP outputs now fail generation when their generated path is not ignored, including scoped MCP config outputs.
- Resolved MCP secret values are redacted before source hash calculation so generated metadata does not encode secret material.
- Existing outside-fence `.gitignore` patterns are now interpreted semantically when deciding whether managed entries are already covered.

### Tests

- Added generator and CLI coverage for MCP env substitution, `.env` and `--env-file` loading, unresolved placeholders, secret safety errors, scoped MCP outputs, deprecated gitignore aliases, and shorthand flags.

## [4.2.2] - 2026-05-26

### Added

- Added a `polyglot-bindings` builtin with Rust-core, native ABI, FFI ownership, cross-language error conversion, and binding parity guidance.

### Changed

- Updated the TypeScript builtin to recommend `oxfmt` with `oxlint`.

### Dependencies

- Updated shared pre-commit hooks, `gh-actions-updater`, `github.com/modelcontextprotocol/go-sdk`, and related Go module dependencies.

## [4.2.1] - 2026-05-21

### Fixed

- **Cursor skill frontmatter is now valid YAML**: generated `.agents/skills/*/SKILL.md`
  files quote skill descriptions, so descriptions containing colons no longer
  fail Codex skill loading.

### Dependencies

- Refreshed pre-commit hook revisions with `prek autoupdate`.
- Updated Go module dependencies with `go get -u ./...` and `go mod tidy`.

## [4.2.0] - 2026-05-21

### Added

- **Manifest-scoped generation cleanup**: `ai-rulez generate` now writes `.generated-manifest.json` under the active config directory and deletes only stale files previously recorded in that manifest. Assistant directories such as `.claude/`, `.codex/`, `.cursor/`, `.gemini/`, `.windsurf/`, `.cline/`, `.agents/`, `.continue/`, `.opencode/`, and `.junie/` are no longer treated as fully owned.
- **Real dry-run output**: `generate --dry-run` and MCP `generate_outputs` dry runs now report planned `create-dir`, `write-file`, and `delete-stale` entries without mutating the filesystem.
- **Custom config directory support**: `generate --config-dir <name>`, `validate --config-dir <name>`, recursive generation, and MCP generation/validation can use configuration roots other than `.ai-rulez/`.
- **Exact config path loading**: positional config paths and `--config <path>` now load that exact file or config directory. Root-level config files fail with a precise directory-layout error when they would otherwise make the parent directory the output root.
- **Scoped subfolder outputs**: new `[[scopes]]` config entries generate subfolder-specific `AGENTS.md` and `CLAUDE.md` files with their own profile and preset selection, keeping root context separate from scoped context.

### Fixed

- **User-owned files no longer get deleted**: hand-written settings, hooks, personal skills, and other files in assistant output directories are preserved during regeneration.
- **CI Taskfile drift**: consolidated on `Taskfile.yml`, removed the lowercase duplicate, and added the workflow-referenced tasks: `test`, `test:platform`, `test:e2e`, `test:e2e:cli`, `test:e2e:mcp`, `test:e2e:integration`, `test:all`, and `test:benchmark`.
- **`task format` no longer walks local caches**: the task now uses `go fmt ./...` instead of formatting every file below the repository root.

### Changed

- `.gitignore` generation now lists generated files individually instead of ignoring whole assistant directories.
- MCP generation and validation now share the same config-loading semantics as the CLI.
- Documentation, schema, and the bundled `ai-rulez` skill now describe custom config directories, scoped outputs, manifest cleanup, and current `[[mcp_servers]]` configuration.

### Dependencies

- Merged Dependabot updates for `github.com/pelletier/go-toml/v2`, `golang.org/x/text`, `github.com/kaptinlin/jsonschema`, and `pymdown-extensions`.

### Tests

- Added regression coverage for preserving user-owned assistant files, manifest-owned stale cleanup, exact config file loading, `--config`, `--config-dir`, dry-run behavior, and scoped subfolder outputs.

## [4.1.6] - 2026-05-03

### Changed

- **Git fetches now use sparse checkout** — `ai-rulez` no longer downloads entire repository archives to resolve installed skills or remote includes. All git operations now run `git clone --depth 1 --filter=blob:none --sparse` and materialise only the required subtree (e.g. `skills/<name>/` or `.ai-rulez/`). This fixes the `"response body too large"` error that occurred when installing skills from large repositories, and dramatically reduces network and disk usage for all remote sources. **Requires git ≥ 2.25** (released January 2020).
- **BLAKE3-based cache invalidation** — the time-based TTL (`.fetch_time` marker, 1-hour for includes) is replaced with a content-driven approach. On every `ai-rulez generate`, a fast `git ls-remote` call checks whether the remote HEAD SHA has changed; cached content is reused when the SHA matches and re-fetched only when it differs. Cached file content is hashed with BLAKE3 and stored in `.cache_meta.json` alongside each cached source. Skills always check the remote (no grace period); `--no-fetch` bypasses all network calls as before.

### Removed

- HTTP archive download path (`downloadAndExtract`, `buildArchiveURL`, `extractTarGz`, `extractZip`): replaced by sparse git clone.
- Duplicate SSH clone helpers (`cloneViaGit`, `cloneViaGitForSkill`): replaced by the shared `sparseClone` primitive in the new `internal/includes/gitops.go`.

## [4.1.5] - 2026-05-02

### Changed

- **Skill resources are no longer concatenated into `SKILL.md`.** A skill's `references/`, `scripts/`, and `assets/` subdirectories are now emitted as separate files under the rendered skill directory, matching the canonical Agent Skills layout used by Claude Code and OpenAI Codex. `SKILL.md` carries a `## Resources` index with relative-path links so the agent can read references on demand (progressive disclosure) instead of paying the full reference cost on every invocation. Reference descriptions are pulled from each file's `description` frontmatter or the first heading.
  - Loader: `internal/includes/skill_source.go::ScanInstalledSkillDir` no longer inlines `references/*.md` (the deleted `readReferences` helper). Local skills under `.ai-rulez/skills/<name>/` now also pick up bundled resources via `internal/config/loader.go::scanSkills` — previously these subdirectories were ignored.
  - Renderer: shared helpers `RenderSkillResourcesIndex`, `SkillResourceOutputs`, `InlineSkillResources` in `internal/generator/presets/skill_resources.go`. Applied across every preset that emits a skill directory (Claude, Codex, Cursor, Cline, Amp, Antigravity, Copilot, Gemini, Junie, Opencode, Windsurf). The single-file `continue.dev` preset keeps the inline-concat behaviour since it has no skill directory to read from.
  - File mode (executable bit on `scripts/*.sh`) is preserved through generation.

### Added

- **`OutputFile.RawContent []byte` and `OutputFile.Mode os.FileMode`** in `internal/config/presets.go` — non-nil `RawContent` routes the file through a verbatim-write path that skips the AI-RULEZ banner, content/source-hash injection, and trailing-newline normalisation. Used for skill resource files where any added marker would corrupt the payload (Python scripts, binary assets) or break tooling that hashes the file.
- **Idempotency on the raw-write path**: `internal/generator/generator.go::writeOutput` now compares existing bytes plus mode and skips the write/chmod when both match, so unchanged bundled assets don't dirty the working tree on every regeneration.
- **`SkillResource` type and `ContentFile.Resources`** in `internal/config/types.go` carry `Kind` (`references`/`scripts`/`assets`), forward-slash-normalised `RelPath`, raw `Content` bytes, file `Mode`, and an optional `Description`.
- **Stale-file cleanup for resource subdirectories**: `SkillResourceOutputs` emits each parent directory (including nested ones) as an `IsDir` output so `cleanManagedDirs` walks them on regeneration. Without this, a deleted `references/old-api.md` would persist forever in the rendered skill tree.
- **Resource content is included in the source hash** (`writeContentFiles`), length-prefixed (`|res=<kind>:<relpath>:len=<n>:<bytes>`) so a reference body cannot spoof the inter-record delimiter to fake a second resource.

### Security

- **Symlink guard in `internal/config/skill_resources.go`**: the resource loader uses `os.Lstat` on each kind directory and checks `d.Type()&os.ModeSymlink` on every walked entry. A malicious installed skill cannot exfiltrate host files (e.g. `references/evil.md → /etc/passwd`) or replace a kind directory with a symlink to an attacker-controlled tree. Symlinks are skipped with a `WARN` log.
- **Defensive walk-up guard in `SkillResourceOutputs`**: an absolute `RelPath` (which `LoadSkillResources` cannot produce, but a future caller might) used to put the parent-directory walk into an infinite loop on `filepath.Dir`. Now `filepath.IsAbs` is checked before the walk.

### Tests

- `internal/config/skill_resources_test.go` — `LoadSkillResources` covering canonical layout, nested paths, frontmatter description extraction, scripts/assets handling, symlink-to-file rejection, symlinked-kind-directory rejection, symlinked subdirectory rejection, dangling symlinks, executable-bit preservation.
- `internal/generator/presets/skill_resources_test.go` — `RenderSkillResourcesIndex`, `SkillResourceOutputs`, `InlineSkillResources`, `referenceDisplayName`, plus the absolute-path infinite-loop guard with a 2 s timeout sentinel.
- `internal/generator/presets/claude_test.go::TestClaudePresetGenerator_PreservesSkillResourcesLayout` — end-to-end assertion that references stay separate from `SKILL.md`, scripts/assets round-trip via raw bytes, and the resource index is rendered.
- `internal/generator/generator_test.go` — raw-write happy path (text, binary, parent-dir creation, mode preservation, mode fallback), raw-write idempotency using `os.Chtimes` sentinel mtime, mode-only changes still rewrite, content-only changes still rewrite. `TestGenerator_CleansStaleSkillResource` verifies a deleted reference is swept on regeneration. `TestComputeSourceHash_IncludesSkillResources` and `TestComputeSourceHash_ResistsResourceDelimiterCollision` lock down the source-hash format.
- Updated `internal/includes/skill_source_test.go` and `internal/includes/skill_resolver_test.go` to assert on `Resources` rather than the deprecated inline-concat behaviour.

### Migration

No config or schema change. Existing skills regenerate on next `ai-rulez generate`. Generated `SKILL.md` files become smaller (reference bodies move out); new sibling files appear under each skill directory.

## [4.1.4] - 2026-05-01

### Fixed

- **Flaky MCP e2e test client**: the per-test `MCPClient` was a single-line-per-request stdio reader with no JSON-RPC id matching, no notification handling, a fresh reader goroutine per call, and the default 64 KiB `bufio.Scanner` buffer. The `tools/list` response is already ~18 KiB on a single line and grows with the tool surface; any server-emitted notification interleaved with a response would be misparsed as the response and orphan the real one. After bumping `modelcontextprotocol/go-sdk v1.5.0 → v1.6.0` in v4.1.3, this manifested as `MCP request timed out` flakes on cold runs (`tests/e2e/testutil/mcp_client.go`).

### Changed

- **MCP e2e test client rewritten** for correctness:
  - One persistent reader goroutine demuxes stdout into per-request response channels keyed by JSON-RPC id, so notifications cannot be misparsed as responses.
  - Notifications (frames without an `id`) are silently discarded.
  - JSON-RPC id key normalized via re-marshal so request and response sides format the same way (encoding/json's float64 round-trip for large nanosecond ids was the latent gotcha).
  - `bufio.Scanner` buffer raised to 1 MiB.
  - Per-RPC timeout standardized at 30 s with explicit `t.Fatalf` showing the method name and reader error on server-side exit.
  - `Close()` now waits for the reader to drain so its goroutine doesn't outlive the test.
- **Reverted v4.1.3's blind 5 s → 30 s timeout bump** as the cure: that bump only masked the underlying client fragility. With the rewrite the timeout is no longer the load-bearing fix.

## [4.1.3] - 2026-04-30

### Fixed

- **`generate --recursive` performance and robustness**: a recursive run on a polyglot monorepo could take ~2 minutes and abort on a single broken symlink (e.g. a stale Rust `target/debug/deps/lib*.rlib`). Two underlying defects in `cmd/commands/generate.go`:
  - The walker had no skip list — it descended into `target/`, `node_modules/`, `.venv/`, `vendor/`, `dist/`, `.git/`, etc.
  - The walk callback re-returned every `lstat` error fatally, so one bad symlink killed the entire run.
- **Recursive discovery now ignores v2 flat configs**: only `.ai-rulez/config.{toml,yaml,yml,json}` is discovered by `--recursive`. The legacy v2 `ai-rulez.yaml` discovery path is removed (single-config `generate <file>` is unaffected).

### Added

- **Shared skip helper** at `internal/walkutil` covering VCS metadata, build outputs (`target`, `node_modules`, `vendor`, `dist`, `build`, `out`, `obj`, `bin`), language toolchain caches (`.venv`, `__pycache__`, `.tox`, `.gradle`, `.mvn`, …), editor caches, and any hidden directory other than `.ai-rulez`/`.github`. Reused from `cmd/commands/generate.go`, `internal/mcp/handlers/project.go`, and `internal/agents/context.go` so the same pruning applies to every recursive walk.
- **Shared rule library detection**: a directory named `ai-rulez/` (no leading dot) that itself contains `config.{toml,yaml,yml,json}` at its root is treated as a shared library. Its entire subtree is pruned during recursive discovery, so nested `.ai-rulez/` module configs that exist only for inclusion by consumers are no longer (re-)generated for.
- **Parallel multi-config processing**: `generate --recursive` now processes discovered configs concurrently with a `runtime.NumCPU()`-bounded worker pool. Each config has its own working directory and produces independent output.
- **Concurrency-safe include cache**: `internal/includes` now serializes refresh of any one cache directory with a per-cacheDir mutex (double-checked, lock-free fast path) so parallel callers targeting the same shared include never race on `RemoveAll`/extract. Applies to both `GitSource.Fetch` and `SkillGitSource.Fetch`.
- **Process-level scanned-tree memoization**: `ScanContentTree` results for a given cached `.ai-rulez/` directory are reused across consumers within the same process. In a monorepo where 18 configs each include the same 5 shared libraries, this cuts 90 redundant tree scans down to 5. Per-consumer include filters still apply via `filterContent`, which returns a new tree without mutating the cached one.

### Performance

- **`kreuzberg-dev` (24 `.ai-rulez/` dirs, 18 actual consumer configs, 5 shared library modules):** `generate --recursive --update-gitignore` went from ~2 minutes (failing on a stale `.rlib` symlink) to ~1 second steady-state. Cold full-tree generation completes in ~12 s.

### Tests

- `internal/walkutil/skip_test.go` — covers the shared skip predicate.
- `cmd/commands/generate_recursive_test.go` — fixture-driven test covering pruned dirs, broken-symlink resilience, library-skip, and config-format priority (TOML over YAML over JSON).
- `internal/includes/fetch_concurrency_test.go` — fetch-lock identity, concurrent access, scanned-tree cache round-trip / invalidation, race-detector stress.
- `tests/e2e/cli/recursive_test.go` — end-to-end suite asserting (a) walker pruning of `node_modules`/`target`/`.venv`/`vendor`/`.cache`/`build`, (b) shared rule library subtree is skipped, (c) parallel and `GOMAXPROCS=1` runs produce byte-identical outputs.

### README

- New collapsible Installation section covering Homebrew, npx, npm -g, uvx, uv tool, pip/pipx, pre-commit hook, and lefthook setup.

## [4.1.2] - 2026-04-30

### Added

- **Per-subagent reasoning effort for Codex**: Codex subagent TOML files (`.codex/agents/<id>.toml`) now emit `model_reasoning_effort` when an effort is resolved for that agent. Per-agent metadata wins over `.codex/config.toml`, which still carries the global default. Tracks the schema documented at <https://developers.openai.com/codex/subagents>.
- **Per-subagent reasoning effort for Opencode**: Opencode agent files (`.opencode/agents/<id>.md`) now emit a `reasoningEffort` frontmatter field. Resolution: per-agent metadata → `defaults.effort_by_preset["opencode"]` → `defaults.effort`. `xhigh` and `max` map to `high` (Opencode tops at `high`); `inherit` is dropped.

### Changed

- Updated the per-preset effort support matrix in `docs/configuration.md` to reflect Codex per-agent support and Opencode per-agent support.

## [4.1.1] - 2026-04-30

### Added

- **Reasoning effort across multiple providers**: extends the v4.1.0 Claude-only effort support to Codex, Amp, and Windsurf.
  - **Codex**: emits `.codex/config.toml` with `model_reasoning_effort` when an effort resolves. Global setting (Codex doesn't accept per-agent effort).
  - **Amp**: emits `.amp/settings.json` with `amp.anthropic.effort`. Global setting; `xhigh` maps to `high` (Amp tops at `high`/`max`).
  - **Windsurf**: emits `reasoning_effort` per-agent in `.windsurf/agents/<id>.md` frontmatter. `max` maps to `high`.
  - **Claude**: refactored to share the same resolver path; behavior unchanged.
- **`defaults.effort_by_preset`**: per-preset overrides that beat `defaults.effort`. Per-agent metadata still wins where the preset supports it. YAML/TOML key validated against the registered preset list.
- **MCP `update_config` `default_effort_by_preset` parameter**: object-typed argument that lets MCP clients set or clear per-preset overrides. `read_config` always returns the field (possibly empty) for stable read-modify-write loops.

### Notes

- Cursor, Copilot, Gemini, Junie, Opencode, Antigravity, Cline, and Continue.dev expose effort behind UI toggles or in user-managed config files we don't generate. ai-rulez deliberately skips emission for them — guard tests lock that in. Configure effort in those tools' own settings instead.
- See `docs/configuration.md` for the full per-preset mapping table.

## [4.1.0] - 2026-04-29

### Added

- **Reasoning effort on Claude Code subagents**: agent frontmatter now accepts an `effort` field (`low` | `medium` | `high` | `xhigh` | `max` | `inherit`) that ai-rulez emits into `.claude/agents/<name>.md`. Maps directly to Claude Code's adaptive thinking spec — available levels depend on the model.
- **`defaults.effort` in `config.yaml` / `config.toml`**: top-level project default that propagates to every generated subagent which doesn't declare its own `effort`. Resolution order: per-agent → `defaults.effort` → omit.
- **MCP `update_config` `default_effort` parameter**: lets MCP clients set or clear the project-wide default. `read_config` now always returns `default_effort` (possibly empty) so read-modify-write loops have a stable contract.
- **Validation** for the new value set across config load, MCP `update_config`, and `ai-rulez validate` — invalid values fail with an actionable message naming the field and the offending value.

### Notes

- Other presets (Cursor, Windsurf, Copilot, Gemini, Antigravity, etc.) silently skip the `effort` field — they have no native equivalent yet. No-leak tests lock that in.
- Claude Code's session-level effort remains a runtime setting (`/effort` slash command) — there is no static surface to render it into, so this release covers subagents only.

## [4.0.8] - 2026-04-27

### Fixed

- **Generation was non-deterministic across runs**: `content.Domains` is a Go map, and every preset that flattened domain rules/context/skills/agents/commands iterated it in randomized order. Two consecutive `generate` runs with identical sources produced different rule orderings in `CLAUDE.md`, `.github/copilot-instructions.md`, and every other multi-rule output, breaking pre-commit hook idempotency. Domain iteration is now sorted by name (`internal/generator/presets/helpers.go`).
- **`tools` field corrupted into a Go slice string**: `Metadata.Extra map[string]string` could not hold YAML sequences — `tools: [Read, Grep, Glob]` was stringified via `fmt %v` to `"[Read Grep Glob]"` and emitted as `tools: '[Read Grep Glob]'`. Added typed `Tools`, `Skills`, `Keywords` fields on `Metadata`; YAML now round-trips as proper sequences in agent frontmatter across all presets (claude, amp, antigravity, cline, copilot, gemini, junie, windsurf).
- **`mcp` preset rejected by validation**: `internal/generator/presets/mcp.go` registered an `mcp` preset generator, but `internal/config/types.go` `builtInPresets` didn't list it — configs that included `mcp` in their preset array failed with `unknown built-in preset: "mcp"`. Added to the map.
- **Skip-on-content-hash never fired for files with both frontmatter and a banner**: windsurf rule files have YAML trigger frontmatter prepended to the standard generated-file banner. `stripHeader` only stripped one layer, so the banner's per-run timestamp leaked into the body hash and caused unnecessary rewrites every run. Now strips both layers.

### Added

- **`Source-Hash` header line**: alongside the existing `Content-Hash`, every generated file now embeds a blake3 hash covering all profile-relevant inputs (config metadata, content tree, MCP servers, plus a generator schema version constant). The skip decision in `writeOutput` requires both hashes to match the values stored in the existing file — never re-hashes the on-disk body, so it's robust to formatters that may modify generated files post-write.
- **Hash injection for YAML-frontmatter files**: skill and agent files (`.claude/skills/*/SKILL.md`, `.opencode/agents/*.md`, etc.) had no header to inject hashes into and were rewritten every run. Hashes now go in as YAML comment lines (`# Content-Hash:` / `# Source-Hash:`) inside the frontmatter, where YAML parsers ignore them.

### Changed

- **Sort everything alphabetically by name**: scanner's `sortByPriority` replaced with `sortByName`; merged content slices re-sorted in `combineContentFiles`; typed list metadata (`Tools`/`Skills`/`Keywords`) sorted on load. Priority is preserved as metadata in the rule body and rendered next to the rule name. This is a behavior change — projects with mixed-priority rules will see one round of reordered output.
- **Output normalized to a single trailing newline** at write time, so `end-of-file-fixer` and similar formatters don't modify files post-generation.

## [4.0.7] - 2026-04-27

### Fixed

- **`.mcp.json` not asserted in managed gitignore fence**: `cursor`, `copilot`, and the auto-`mcp` preset all emit `.mcp.json` when MCP servers are configured, but no regression test confirmed the path actually landed in the `# BEGIN ai-rulez` block. Coverage added; behavior verified end-to-end across all preset combinations.
- **Cross-fence gitignore duplication**: when a user already had a pattern (e.g. `.cursor/`, `CLAUDE.md`) listed manually outside the managed block, regenerate added the same line _inside_ the fence too. The writer now skips any pattern already present outside the fence.
- **`--debug` flag did nothing**: registered on `RootCmd` but never propagated to the logger singleton. Added `logger.SetLevel` and a `PersistentPreRun` that lowers the level to `DEBUG` when `--debug` is set (and to `ERROR` for `--quiet`).
- **`--update-gitignore` flag did nothing**: declared on `generate` but never read. Now forces `cfg.Gitignore = true` regardless of the config file value, matching the help text.

### Changed

- **Demoted intentional-behavior warnings to debug**: scanner's `domain X file overrides root file` and `multiple domains have same file` were `WARN`-level on every legitimate domain override (documented design, not user error). Generator's `Output path conflict` likewise fired any time `cursor`+`copilot`+auto-`mcp` shared `.mcp.json` (also expected). All three are now `DEBUG`.
- **Quieter generation output**: `Processing commands for Claude preset`, per-command `Checking command` / `Including command`, and `Scanned commands directory` are now `DEBUG`. Run with `--debug` to see them again.

## [4.0.6] - 2026-04-25

### Fixed

- **MCP schema rejected V4 configs**: `ai-rules-mcp.schema.json` only allowed version `"3.0"` — V4 configs failed validation. Now accepts both `"3.0"` and `"4.0"`.
- **Generated file headers hardcoded `config.yaml`**: preset generators always wrote `Source: .ai-rulez/config.yaml` in output headers, even for TOML or JSON configs. Headers now reflect the actual config filename.

### Changed

- **Removed stale V3 naming across codebase**: `ValidateV3()` renamed to `Validate()`, `isV3ConfigFile()` to `isConfigFile()`, `DetectConfigVersion` returns `"dir"` instead of `"v3"`, test fixtures and helpers renamed to version-neutral names.
- **Added `IsV4()` method** to `Config` for symmetry with `IsV3()`.

## [4.0.5] - 2026-04-25

### Fixed

- **Config discovery missing TOML**: `FindConfigFile` (used by MCP handlers) only searched for YAML/YML configs, ignoring `config.toml` (the V4 default) and `config.json`. TOML is now checked first.
- **Recursive generate missed TOML configs**: `isV3ConfigFile` (used by `generate --recursive` and pre-commit hooks) did not match `config.toml` or `config.json`, so projects using the V4 default were silently skipped.

## [4.0.4] - 2026-04-25

### Fixed

- **Pre-commit hooks broken for V3/V4 users**: file trigger patterns in `.pre-commit-hooks.yaml` only matched V2-style filenames — hooks never fired when `.ai-rulez/` directory content changed. Updated to `^\.ai-rulez/`.
- **Stale versions across packages**: default download version in `run-ai-rulez.sh` was `v3.0.0`, `officialPreCommitRev` in `setup.go` was `v2.4.3`, lefthook glob was V2-only. All updated.
- **Init templates generated old config version**: YAML and JSON templates used `version: "3.0"` while TOML correctly used `"4.0"`. All formats now default to `"4.0"`.
- **PyPI wrapper version drift**: `release/pypi` `__version__` was stuck at `3.14.2`, causing binary download mismatches.
- **Stale `ErrInvalidVersion` sentinel**: error message only mentioned `3.0`, now includes `4.0`.

### Added

- **Taskfile**: added `Taskfile.yml` with `setup`, `update`, `upgrade`, `set-version`, `build`, `test`, `lint`, `check`, and `clean` tasks. `set-version` updates all 8 version locations in one command.
- **Hook unit tests**: added `internal/hooks/hooks_test.go` and `setup_test.go` covering detection, all three hook systems (lefthook, pre-commit, husky), idempotency, legacy pruning, and error paths.

## [4.0.3] - 2026-04-24

### Fixed

- **Gitignore cleanup**: shared directories (e.g. `.github/`) now use subdirectory patterns (`.github/agents/`, `.github/skills/`) instead of listing every individual file. Nested paths deduplicated automatically.
- **Stale binary**: builtin agents were generated by `GeneratePresets` but not written to disk when using a stale binary. Confirmed working with fresh build.

## [4.0.2] - 2026-04-24

### Added

- **Builtin agents**: code-reviewer, test-writer, security-auditor, docs-writer, devops-engineer, release-engineer — specialized agents shipped with the tool, ready to use as subagents.
- **New rules** (adapted from superpowers patterns): verification-before-completion, systematic-debugging, testing-anti-patterns.
- **Strengthened TDD rule**: iron law enforcement — wrote code before the test? Delete it, start over, no exceptions.

### Changed

- **README redesigned**: value-first structure showing builtin capabilities, agents, and full development workflow.
- **Package descriptions updated** across npm, PyPI, and GitHub to reflect complete workflow capabilities.

## [4.0.1] - 2026-04-24

### Added

- **New builtins**: `cicd` (pipeline standards, GitHub workflow), `docker` (container best practices), `observability` (logging, metrics, health checks).
- **New ai-governance rules**: `no-ai-signatures` (no AI attribution in commits/PRs/code), `agent-workflow` (subagent delegation with mandatory critical review), `communication-style` (concise, no fluff/emojis/checklists).
- **New testing rule**: `tdd-workflow` (TDD red-green-refactor, test type taxonomy).
- **New code-quality rule**: `anti-patterns` (magic numbers, global state, composition over inheritance).
- **Auto-include expanded**: `code-quality`, `testing`, `git-workflow`, `security`, `token-efficiency` are now auto-included by default alongside `ai-governance` and `agent-delegation`.

### Fixed

- **`migrate v4` preserves MCP servers** from legacy `mcp.yaml` files — previously lost during migration.

### Changed

- **`token-efficiency/task-runner`** enriched with standard task naming conventions and lock file requirements.

## [4.0.0] - 2026-04-23

### Breaking Changes

- **TOML is the default config format**: `ai-rulez init` now generates `config.toml` instead of `config.yaml`. Existing YAML configs continue to work.
- **MCP servers are inline**: MCP servers are now configured in your main config file under `[[mcp_servers]]` instead of a separate `mcp.yaml`. Legacy `mcp.yaml` files are still loaded with a deprecation warning.
- **V2 compatibility removed**: All V2 config types, migration code, validator, and generators have been removed. V3 YAML configs remain fully supported.
- **Deprecated features removed**: `CompressionConfig`, `--skip-mcp` flag, `--auto-migrate` flag, `migrate v3` command.

### Added

- **Agent generation for all presets**: Amp, Windsurf, Cline, and Continue.dev now generate agent files with YAML frontmatter.
- **Context rendering for per-file presets**: Cursor, Windsurf, and Cline now render context as rule-like files.
- **Claude MCP & plugins output**: `.claude/settings.json` (MCP servers) and `.claude/plugins.json` (plugin declarations).
- **Cursor/Copilot MCP output**: `.mcp.json` generated when MCP servers are configured.
- **Codex plugins & commands**: `.codex/plugins.json` and `.codex/commands/` output.
- **Copilot command generation**: `.github/commands/` output for Copilot.
- **Gemini/Antigravity user MCP servers**: Settings.json now includes user-configured servers alongside the hardcoded ai-rulez server.
- **TOML config support**: Config loader tries TOML first, then YAML, then JSON.
- **Plugins and marketplaces**: New `[[plugins]]` and `[[marketplaces]]` config sections for declaring tool extensions.
- **`migrate v4` command**: Converts `config.yaml` to `config.toml`, inlines MCP servers, removes old files.
- **Backward-compatible `mcp.yaml` loading**: Legacy separate MCP files are loaded with a deprecation warning.

### Changed

- **All V3 types renamed**: `ConfigV3` → `Config`, `ContentTreeV3` → `ContentTree`, `OutputFileV3` → `OutputFile`, etc.
- **Schema files renamed**: `ai-rules-v3.schema.json` → `ai-rules.schema.json`, `ai-rules-v3-mcp.schema.json` → `ai-rules-mcp.schema.json`.
- **Schema updated**: Accepts version `"3.0"` or `"4.0"`, adds `plugins` and `marketplaces` fields, removes deprecated `compression`.
- **Documentation migrated to Zensical**: Replaces MkDocs with Zensical for documentation site generation.
- **All documentation updated for V4**: TOML examples, inline MCP, plugins/marketplaces, updated CLI reference.

### Removed

- V2 config types, loader, migration tool, validator, and generators (~3000 lines).
- V1 and V2 JSON schemas.
- V2 template rendering engine and builtin templates.
- `CompressionConfig` (deprecated no-op).
- Separate MCP file loading functions (replaced by inline config).
- MkDocs configuration (`mkdocs.yaml`).
- Generated documentation site (`site/`).

## [3.14.2] - 2026-04-20

### Fixed

- **Critical: generator deleting non-generated files in `.github/`**: `cleanManagedDirs` was treating `.github/` as a fully managed directory, deleting workflows, CODEOWNERS, issue templates and other user content during `generate`. Now skips shared directories that contain both generated and non-generated content.

## [3.14.1] - 2026-04-20

### Fixed

- **CI: missing Taskfile tasks**: Added `test:e2e`, `test:e2e:cli`, `test:e2e:mcp`, `test:e2e:integration`, `test:platform`, and `test:all` tasks that the CI/E2E workflows referenced but didn't exist.
- **CI: golangci-lint-action**: Pinned to `@v7` (resolves `@latest` lookup failure), use `version: latest` for the binary.
- **CI: stale test expectations**: Fixed `TestInitExistingConfig` (unset CI env to test non-interactive mode) and `TestValidateFailsWhenSkillDescriptionMissing` (updated to match 3.13.1 behavior change where missing description is a warning, not error).
- **Windows test failures**: Fixed path separator issues in `TestAmpPresetGenerator_Generate_WithSkills` and `TestClinePresetGenerator_Generate_WithSkills` using `filepath.ToSlash`.
- **Stale comment**: Fixed `git.go` cache directory comment (was `.ai-rulez/.remote-cache/`, actual is `~/.cache/ai-rulez/includes/`).

## [3.14.0] - 2026-04-20

### Added

- **Antigravity preset**: New `antigravity` built-in preset generating `GEMINI.md`, `.agents/settings.json`, and skills/agents to `.agents/skills/` and `.agents/agents/`.
- **Gemini skills and agents**: Gemini preset now generates skill files to `.agents/skills/{id}/SKILL.md` and agent files to `.agents/agents/{name}.md` with YAML frontmatter.
- **Cursor agents**: Cursor preset now generates agent files to `.agents/agents/{name}.md` with YAML frontmatter (name, description, model, readonly, is_background).
- **Cursor skills moved to `.agents/`**: Cursor skills output moved from `.cursor/skills/` to `.agents/skills/` following the cross-agent standard.
- **Codex subagents**: Codex preset now generates agent files to `.codex/agents/{name}.toml` in TOML format (name, description, developer_instructions).
- **AMP skills**: AMP preset now generates skill files to `.agents/skills/{id}/SKILL.md`.
- **Windsurf skills**: Windsurf preset now generates skill files to `.windsurf/skills/{id}/SKILL.md`.
- **Copilot skills and agents**: Copilot preset now generates skill files to `.github/skills/{id}/SKILL.md` and agent files to `.github/agents/{name}.agent.md` with YAML frontmatter.
- **Cline skills**: Cline preset now generates skill files to `.cline/skills/{id}/SKILL.md`.
- **Junie skills and agents**: Junie preset now generates skill files to `.junie/skills/{id}/SKILL.md` and agent files to `.junie/agents/{name}.md` with YAML frontmatter.
- **OpenCode skills and agents**: OpenCode preset now generates skill files to `.opencode/skills/{id}/SKILL.md` and agent files to `.opencode/agents/{name}.md` with YAML frontmatter.
- **Output conflict detection**: Generator now warns when multiple presets write to the same file path, deduplicates directory entries, and uses last-write-wins for file conflicts.
- **Vite+ builtin**: New `vite-plus` builtin for the unified TypeScript toolchain (oxlint, oxfmt, vitest, rolldown/tsdown, task caching).
- **MCP read tools**: New `read_rule`, `read_context`, `read_skill` MCP tools for reading file content without filesystem fallback.
- **MCP `working_directory` parameter**: All MCP CRUD and project tools now accept an optional `working_directory` parameter for polyrepo support.
- **MCP enum constraints**: `priority` and `merge_strategy` fields now use JSON Schema `enum` constraints for better LLM tool use.
- **MCP `read_config` / `update_config`**: New tools for reading and updating `config.yaml` fields (name, description, builtins, gitignore) via MCP.
- **MCP `dry_run` / `recursive`**: `generate_outputs` now accepts `dry_run` (preview mode) and `recursive` (walk subdirectories) parameters.
- **MCP annotations**: All tools now have `readOnlyHint`/`destructiveHint` annotations so clients can implement appropriate confirmation UX.
- **`builtins show` command**: New `ai-rulez builtins show <name>` CLI command and `show_builtin` MCP tool to inspect builtin domain contents (rules, context, skills with priorities).
- **Content hash**: Generated files include a `Content-Hash: blake3:<hex>` in the header. Files are skipped when the hash matches, eliminating timestamp-only diffs.
- **Include cache TTL**: Git-based includes are cached for 1 hour instead of re-fetched every run. New `--no-fetch` flag for offline generation.

### Changed

- **Rust builtin**: Added Rust API Guidelines (naming conventions, trait implementations, type safety, builder pattern, sealed traits, rustdoc standards) and Rust Design Patterns reference.
- **MCP atomic updates**: `update_rule`, `update_context`, `update_skill` now use atomic overwrite (temp+rename) instead of delete-then-create, preventing data loss on write failure.
- **MCP `with_agents` parameter**: `init_project` now creates `.ai-rulez/agents/` directory when `with_agents` is true (previously silently ignored).
- **MCP `list_context` unified**: Merged `list_context` and `list_contexts` into a single enriched endpoint returning summaries.
- **MCP list metadata**: `list_rules`, `list_context`, and `list_skills` now populate `priority` and `targets` fields from YAML frontmatter.
- **CLAUDE.md inlines all content**: Local project rules and contexts are now inlined in CLAUDE.md instead of using `@path` references, matching all other presets.
- **Gitignore idempotent updates**: `.gitignore` now uses fenced `# BEGIN ai-rulez` / `# END ai-rulez` markers. Repeated `generate` runs replace the block instead of appending duplicates. Old-style `# AI Rules generated files` headers are auto-migrated.

## [3.13.1] - 2026-04-19

### Fixed

- **Frontmatter parsing**: Fall back to raw map parsing when direct YAML unmarshal fails (e.g., SKILL.md files with nested `metadata:` objects). Nested values are stringified for the Extra map.
- **Skill description validation**: Downgraded missing description from a fatal error to a warning. Uses skill name as fallback description instead of failing generation.
- **Cache directory consistency**: Unified all cache locations to `~/.cache/ai-rulez/` (XDG convention) instead of mixing `os.UserCacheDir()` (`~/Library/Caches` on macOS) with `~/.cache/`.

### Changed

- **Module structure**: Restructured shared modules to use `.ai-rulez/` subdirectories, enabling remote includes via GitHub URLs without `local_override`.
- **mdformat exclusion**: Excluded `.ai-rulez/` directories from mdformat pre-commit hook to prevent YAML frontmatter destruction.
- **Enriched agents**: Improved devops-engineer, docs-writer, polyglot-architect, and code-reviewer agents with more detailed guidance.

## [3.13.0] - 2026-04-19

### Added

- **Agent delegation builtin**: New `agent-delegation` auto-included builtin that renders an "## Agents" section in generated outputs (CLAUDE.md, AGENTS.md, GEMINI.md) listing all available subagents with their descriptions and delegation/parallelization instructions. Disable with `builtins: ["!agent-delegation"]`.
- **New binding builtins**: `jni-rs` (Rust-Java/JVM), `extendr` (Rust-R), `cgo` (Go-C/Rust FFI).
- **Token-efficiency rules**: Three new rules — `batch-operations`, `incremental-approach`, `context-preservation` — expanding the builtin from 2 to 5 rules.

### Deprecated

- **Compression**: The `compression` config option is now a no-op and will be removed in a future version. Existing configs with `compression` will still parse without error but emit a deprecation warning. The compression feature's stopword removal at moderate+ levels stripped meaning-critical words (negations, verbs, prepositions), making generated output ungrammatical and sometimes inverting meaning. Condense content at the source level instead.

### Removed

- **Compression package**: Deleted `internal/compression/` — all compression logic, stopword lists, semantic scoring, and hypernym replacement.

### Changed

- **Language builtins improved**: All 10 language convention files (Rust, Python, TypeScript, Go, Java, Ruby, PHP, Elixir, C#, R) restored to 11-14 bullets each with security scanning tools, benchmarking frameworks, build system guidance, and key language patterns.
- **Binding builtins improved**: Fixed accuracy issues in PyO3 (`Py<T>` deprecation wording), Magnus (build tools), and ext-php-rs (error mapping, GC, async guidance).
- **Universal builtins improved**: Rewrote `output-awareness` with concrete limits, restored `dependency-awareness` per-language tool list, rewrote OWASP context verb-first, sharpened `read-before-write` vs `verify-before-acting` distinction, improved `avoid-duplication` with concrete "three similar lines" guidance.

## [3.12.0] - 2026-04-17

### Added

- **Installed skills**: New `ai-rulez skill install/remove/list` commands for installing named skills from external git repos or local paths. Skills are fetched dynamically at generate time and included in outputs. Config field: `installed_skills` in config.yaml.
- **MCP tools for installed skills**: `install_skill`, `uninstall_skill`, `list_installed_skills` MCP operations.
- **Distributable ai-rulez skill**: `skills/ai-rulez/` folder at repo root with comprehensive SKILL.md and reference docs, installable by other projects.
- **`installed_skills` JSON schema**: Schema validation for the new config section.
- **llms.txt**: Added LLM-friendly documentation index at `docs/llms.txt`.

### Fixed

- **YAML config preservation**: `SaveConfigV3` now uses `yaml.Node` round-tripping to preserve field ordering, comments, and formatting when modifying config (e.g., `skill install`, `include add`). Previously, saving re-marshaled the entire config, losing comments and reordering fields.
- **Stale cache invalidation**: Include and skill caches are now cleared before each fetch, preventing stale data from previous downloads from contaminating results.
- **golangci-lint clean**: Extracted `sourceTypeGit`/`sourceTypeLocal` constants, fixed `gocritic` shadow and named result warnings.

### Changed

- **golangci-lint pinned in CI**: `golangci-lint-action@latest` with `version: v2.11.4` in CI workflow.

## [3.11.5] - 2026-04-15

### Fixed

- **Claude preset: no headers in skills/agents/commands**: Generated header comments (`<!-- AI-RULEZ ... -->`) are no longer emitted in skill, agent, or command files. These files serve as prompts for Claude Code — header comments wasted tokens and injected confusing "DO NOT EDIT" instructions into the agent's system prompt. Headers are now only emitted in CLAUDE.md and equivalent top-level files.

## [3.11.4] - 2026-04-15

### Fixed

- **Claude preset: frontmatter placement**: Skill, agent, and command files now emit YAML frontmatter (`---`) as the first line, before the generated header comment. Previously the HTML comment header was placed before frontmatter, preventing Claude Code from parsing it.
- **Claude preset: skill frontmatter fields**: Skills now include `user_invocable` (false for domain skills, true for commands) and `description` in frontmatter. Removed ai-rulez internal fields (`priority`, `targets`) from Claude output.
- **Include domain profiles (#97)**: `GetContentForProfile` now adds `FromInclude` and builtin domains before profile-specific domains, ensuring included domains are always available regardless of profile configuration. `mergeDomainInstall` now correctly sets `FromInclude=true` on new domains.
- **Non-deterministic output**: `mergeContentFiles` with `baseWins=false` now iterates the include slice instead of a map, producing deterministic file ordering across regenerations.
- **Lint**: Fixed `rangeValCopy` warnings in include CRUD operations and `gofmt` formatting in `IncludeConfig` struct.

### Added

- **`local_override` for includes**: New `local_override` field on include configs allows using a local directory instead of fetching from git. If the local path exists, it is used; if not, the include falls back to the configured git source. Supports the `path` subdir field. Useful for developing shared rules locally before pushing.
- **Minimal headers for skills/agents**: `StyleOverride` field on `TemplateData` allows preset generators to force a specific header style. Claude preset now uses "minimal" headers for skills and agents to reduce token waste.

### Changed

- **Profile domain warnings**: `warnMissingDomainReferences` now emits debug-level (not warn-level) messages when includes are configured and a referenced domain is missing, since the domain may exist in an include that failed to resolve.

## [3.11.3] - 2026-03-29

### Fixed

- Includes: fail fast when includes are configured but the includes resolver isn't registered, preventing silent drops of included domains referenced by profiles.

### Added

- Integration coverage for profiles resolving domains delivered via includes, ensuring included domains stay visible to profile selection.

### Changed

- GolangCI-Lint now tracks the `latest` release in CI and pre-commit instead of a pinned version.

## [3.11.2] - 2026-03-25

### Fixed

- **Gitignore: `.github/` directory no longer ignored**: The gitignore updater was adding `.github/` as a directory-level pattern because the copilot preset creates `.github/copilot-instructions.md`. This caused CI workflows and other `.github/` content to be ignored. Shared directories like `.github` are now excluded from directory-level patterns — only individual generated files inside them are gitignored.
- **Gitignore: no more individual file paths**: Files inside fully-managed directories (e.g. `.claude/skills/foo/SKILL.md`) are no longer added individually to `.gitignore` — the parent directory pattern covers them.

## [3.11.1] - 2026-03-25

### Added

- **R language builtin**: R conventions covering tidyverse style, testthat, roxygen2, CRAN compliance, and extendr/rextendr Rust FFI bindings

## [3.11.0] - 2026-03-25

### Fixed

- **Include domain duplication** (issue #97): Include sources now use `ScanContentTree` instead of `scanner.ScanProfile`, preventing domain content from appearing twice in generated output
- **Claude preset output bloat**: Skill, agent, and command-as-skill files no longer embed all rules and context; only explicitly targeted content is included (reduces `.claude/` from ~13MB to ~440KB on large projects)
- **Stale file cleanup**: Generator now removes orphaned files from managed output directories (`.claude/skills/`, `.claude/agents/`) before writing new output

### Changed

- **Rules rendered as `@` references**: Local project rules in CLAUDE.md now use `@path` lazy-loading references instead of full inlining, matching the existing context rendering pattern. Builtin and included rules remain inlined.
- **Exported `ScanContentTree`**: `config.ScanContentTree()` is now public for use by include sources and external consumers
- Context and rules rendering consolidated into shared `renderContentRef` helper

## [3.10.0] - 2026-03-18

### Fixed

- **Included skills validation**: Frontmatter parser now captures `description` and other extra fields from included skill files via YAML inline tag (issue #96)
- **Included domains in profiles**: Include sources (git and local) now discover and scan domain directories, so consuming projects can reference included domains in profiles (issue #97)

### Changed

- Include domain discovery skips hidden directories (e.g. `.git`) in the `domains/` tree

## [3.9.0] - 2026-03-11

### Added

- Root `.ai-rulez/commands/` documentation files for `build`, `fix`, `lint`, `review`, and `test`
- Repository-managed `.pre-commit-config.yaml` with commit-message linting and `prek`-driven checks

### Changed

- Replaced `lefthook.yaml` with the `prek`/pre-commit toolchain across local workflows, CI wiring, and helper scripts
- Refreshed command-generation, docs-site output, and test fixtures to match the new command docs and hook setup
- Aligned Go/tooling dependencies and CI lint configuration with the current Go `1.26` toolchain

### Fixed

- Codex skill generation now always emits `description` in `.codex/skills/*/SKILL.md`, falling back to the skill ID when needed
- V3 validation now rejects source `SKILL.md` files that omit a non-empty `description`
- Skill import, CRUD creation, and V2 section migration now synthesize valid skill descriptions so generated Codex skills always load
- E2E test binary setup now uses a stable temp path, preventing later suites from inheriting deleted binaries

## [3.8.3] - 2026-03-08

### Fixed

- **GetContentForProfile**: Domain content from includes no longer duplicated into root-level slices; domains are now placed only in the Domains map for proper preset generation
- **Includes resolver**: Added `FromInclude` field to `DomainV3` to track domains originating from external includes

## [3.8.2] - 2026-03-08

### Fixed

- **Claude preset**: Domain skills, agents, and commands are now properly collected and generated (previously only root-level content was processed)
- **Builtins**: `default-commands` builtin (`/iterate`, `/parallelize`) now generates Claude skill files correctly

### Changed

- **Builtin language conventions**: All 9 language builtins expanded with explicit linting toolchain, SAST tools, coverage tools, benchmark tools, and package manager recommendations
  - Rust: added `cargo-llvm-cov`, `cargo deny`, `cargo-machete`, `criterion`, `cargo-flamegraph`, `Cow`/`Arc`/`memchr`/SIMD guidance
  - Python: added `bandit`, `hypothesis`, `uv` lockfile, `hatchling`/`maturin`, `pytest-benchmark`, `py-spy`/`scalene`
  - TypeScript: added `oxlint`, `pnpm` preferred, `tsup`/`esbuild`, `socket.dev`/`snyk` supply chain
  - Go: added `govulncheck`, `gosec`, specific `golangci-lint` linters, `benchstat`
  - Java: added Gradle preference, `google-java-format`, `Error Prone`, `Checkstyle`, `SpotBugs`, `JaCoCo`, `JMH`
  - Ruby: added `rubocop` plugins, `bundler-audit`, `brakeman`, `factory_bot`, `simplecov`
  - PHP: added `PHP-CS-Fixer`, `Psalm`, `roave/security-advisories`, PSR-4 autoloading
  - Elixir: added `excoveralls`, `sobelow`, `mix_audit`, `dialyxir`, `ExDoc`
  - C#: added `StyleCop.Analyzers`, `Roslynator`, `coverlet`, `BenchmarkDotNet`, `ValueTask`
- **Security builtin**: `dependency-awareness` rule expanded with per-language audit tool recommendations
- **Token efficiency builtin**: Description updated from "RTK awareness" to "Output efficiency and task automation"

## [3.8.1] - 2026-03-07

### Changed

- **Token reduction**: Replaced simple compression system with kreuzberg-ported token reduction engine
  - 5 reduction levels: `off`, `light`, `moderate`, `aggressive`, `maximum`
  - Markdown-aware processing preserves headers, lists, tables, and code blocks
  - Stopword removal with language support (English)
  - Sentence scoring and selection for aggressive/maximum levels
  - Semantic token scoring and hypernym compression for maximum level
  - Backward-compatible: old level names (`none`/`minimal`/`standard`) auto-mapped
- **Compression config**: New fields `preserve_markdown`, `preserve_code`, `language`; removed `remove_duplicates`, `use_abbreviations`, `preserve_formatting`

### Fixed

- **Includes**: Flat ai-rulez structure (rules/, context/ directly) now detected at sub-paths, not just at repository root
- **Includes**: `findAIRulezDir()` and `findSourceDir()` both support flat structure at sub-paths for git sources

## [3.8.0] - 2026-03-07

### Added

- **Built-in domains system**: 23 embedded content domains shipped with the binary via `//go:embed`
  - 8 universal domains: `ai-governance` (auto-included), `security`, `git-workflow`, `code-quality`, `testing`, `token-efficiency`, `documentation`, `default-commands`
  - 9 language domains: `rust`, `python`, `typescript`, `go`, `java`, `ruby`, `php`, `elixir`, `csharp`
  - 6 binding domains: `pyo3`, `napi-rs`, `magnus`, `ext-php-rs`, `rustler`, `wasm`
- **`builtins` config field** with flexible syntax:
  - `builtins: true` — enable all built-in domains
  - `builtins: false` — disable all (including auto-includes)
  - `builtins: [rust, python, security]` — enable specific domains
  - `builtins: ["!ai-governance"]` — exclude auto-included domains
- **`ai-rulez builtins list`** CLI command to show available built-in domains (supports `--json`)
- **`/iterate` slash command** (via `default-commands` builtin): instructs LLM to work in implementation/review/adjustment cycles
- **`/parallelize` slash command** (via `default-commands` builtin): instructs LLM to split tasks among subagents
- `BuiltinsConfig` type with custom YAML/JSON marshaling supporting boolean and array formats
- Builtins merge at lowest priority — local content and includes always override builtin content

### Changed

- Schema updated: `builtins` field added, preset enum updated with `codex`, `amp`, `junie`, `opencode`

## [3.7.3] - 2026-02-19

### Fixed

- Publish workflow now resolves Go from `go.mod` for GoReleaser (`go-version-file: "go.mod"`), preventing asset build failures when the module Go version advances

### Changed

- Contribution guide now requires Go `1.26+` and references the correct release workflow file (`.github/workflows/publish.yaml`)

## [3.7.2] - 2026-02-16

### Fixed

- Claude preset skill rendering now respects frontmatter `targets` when embedding rules/context in `.claude/skills/*/SKILL.md`, preventing unrelated content leakage
- npm installer now supports offline/private-registry bundled binaries (`bin/ai-rulez-{os}-{arch}`), using packaged binaries before attempting GitHub release downloads

## [3.7.1] - 2026-02-16

### Fixed

- Windsurf trigger frontmatter now safely YAML-quotes `description` and `glob` values, preventing malformed output when values include special characters
- Windsurf invalid trigger warning now reflects the original unsupported trigger value before fallback

## [3.7.0] - 2026-02-16

### Added

- Contributor credit: merged PR [#83](https://github.com/Goldziher/ai-rulez/pull/83) by [@mnsami](https://github.com/mnsami)

### Fixed

- Includes system now correctly includes agents in merged include content (PR #83)
- Codex skill generation now always writes `description` in `.codex/skills/*/SKILL.md` frontmatter

### Changed

- Bumped Go toolchain target to `1.26` and aligned CI workflows
- Bumped `golangci-lint` to `v2.9.0` across Taskfile, hooks, and CI
- Updated Go, Node, and Python/docs dependencies to latest available versions

## [3.6.1] - 2026-01-07

### Fixed

- Windows binary packaging - now uses `.zip` format instead of `.tar.gz` for Windows releases

## [3.6.0] - 2026-01-05

### Fixed

#### Preset Generator Skills/Commands Inlining Bug

- **Claude**: Removed skills and commands from CLAUDE.md (77% size reduction - 14K lines to 3.2K lines)
  - Skills now only generate to `.claude/skills/{skill-id}/SKILL.md`
  - Commands now only generate to `.claude/skills/{command-id}/SKILL.md`
- **Cursor**: Added missing skills and commands directory support
  - Skills now generate to `.cursor/skills/{skill-id}/SKILL.md`
  - Commands now generate to `.cursor/commands/{command}.md` (was `.cursor/rules/cmd-*.mdc`)
- **Codex**: Removed skills inlining from AGENTS.md
  - Skills now generate to `.codex/skills/{skill-id}/SKILL.md`
  - AGENTS.md only contains Rules and Context
- **Gemini**: Removed skills inlining from GEMINI.md
- **Copilot**: Removed skills inlining from `.github/copilot-instructions.md`
- **AMP**: Removed skills inlining from AGENTS.md
- **OpenCode**: Removed skills inlining from AGENTS.md
- **Junie**: Removed skills inlining from `.junie/guidelines.md`

### Changed

- All preset generators now correctly separate skills into dedicated directories
- Main preset files (CLAUDE.md, GEMINI.md, etc.) only contain Rules and Context
- Skills are lazily loaded from separate files, reducing prompt token usage

## [3.5.0] - 2026-01-04

### Added

#### V3-Native Command System

- File-based slash commands in `.ai-rulez/commands/` directory with YAML frontmatter
- Profile-aware commands (root + domain-specific commands)
- Commands generate to preset-specific formats:
  - Claude: `.claude/skills/{command-name}/SKILL.md`
  - Cursor: `.cursor/rules/cmd-{name}.mdc`
  - Continue.dev: Entries in `.continue/prompts/ai_rulez_prompts.yaml`
  - Support for all 18 presets
- Command metadata: name, aliases, description, usage, shortcut, priority, category, targets
- V2 command migration support via `ai-rulez migrate v3`

#### Prompt Compression

- Configurable compression levels: none, minimal, standard, aggressive
- Simple optimizations without external dependencies:
  - Whitespace removal (trailing spaces, excessive blank lines)
  - Priority label compaction
  - Abbreviations (aggressive mode)
- Context optimization: summaries with @ links instead of full content (34% size reduction)
- Compression stats logging during generation

#### Context File Optimization

- Required `summary` field in context frontmatter for concise descriptions
- Context rendered as summaries with @ links to full files
- MCP `list_contexts` tool added to list context files with names and summaries
- Enables agents to fetch full context only when needed

### Changed

- Remote includes cache moved from `.remote-cache/` to system cache directory
  - macOS: `~/Library/Caches/ai-rulez/includes/`
  - Linux: `~/.cache/ai-rulez/includes/`
  - Windows: `%LocalAppData%/ai-rulez/includes/`
- Context files now require `summary` field in frontmatter
- CLAUDE.md and other presets significantly smaller (9.5K vs 14K, 34% reduction)

### Fixed

- Include system now properly merges commands from remote sources
- Scanner properly scans commands in root and domain directories
- Default profile now includes commands in content tree

## 3.4.1 - 2026-01-03

### Added

- SSH git clone support for private repositories - automatically uses `git clone` for SSH URLs (`git@...`, `ssh://...`)
- Support for self-hosted GitLab instances and other GitLab-compatible git servers
- Support for repositories where root IS the ai-rulez structure (no nested `.ai-rulez/` directory)
- Automatic detection of repository structure (standard vs root-level)

### Changed

- Git includes now use native SSH cloning when SSH URLs are detected, leveraging existing SSH key configuration
- Improved git include fetching to skip `.git` directory when copying repository content

### Documentation

- Added comprehensive SSH cloning documentation in docs/includes.md
- Added repository structure support documentation
- Added self-hosted GitLab examples and requirements

## 3.4.0 - 2026-01-03

### Added

- Configurable header styles for generated files (detailed, compact, minimal)
- CLAUDE.md generation to claude preset
- Enhanced headers with AI-RULEZ explanation, folder structure, and MCP server usage instructions
- Markdown formatting support using goldmark and goldmark-markdown
- Markdown processor utilities to normalize embedded content (strip duplicate H1 headings, normalize blank lines)
- Markdownlint configuration for generated files
- Comprehensive documentation for header configuration in docs/configuration.md

### Changed

- All 11 presets now include enhanced headers with AI agent instructions
- Generated markdown files now pass markdownlint validation
- Headers now explain what ai-rulez is, the .ai-rulez folder structure, and how to use the MCP server
- Embedded content processing removes duplicate headings and normalizes formatting

### Dependencies

- Added github.com/yuin/goldmark v1.7.13
- Added github.com/teekennedy/goldmark-markdown v0.5.1

## 3.3.2 - 2026-01-02

### Fixed

- SSH git URL conversion in includes system - now properly converts SSH URLs to HTTPS for archive downloads
- Added support for multiple SSH URL formats: `git@host:owner/repo.git`, `ssh://git@host/owner/repo.git`
- Updated validation to accept SSH URLs alongside HTTP/HTTPS URLs

### Documentation

- Added comprehensive examples for Git includes with SSH and HTTPS URLs in README
- Updated includes documentation with supported Git URL formats and include options

## 3.3.1 - 2025-12-31

### Fixed

- SSH git URL detection in includes system - now properly detects `git@host:path` format URLs
- Previously only HTTP/HTTPS URLs were recognized, causing SSH git URLs to be treated as local paths

## 3.3.0 - 2025-12-31

### Fixed

- Complete agents support in includes system
- Add agents scanning support to includes and scanner

### Changed

- Bump actions/cache from 4 to 5
- Bump actions/upload-artifact from 4 to 6

## 3.2.2 - 2025-12-28

### Added

- Init now creates root and domain agent directories by default
- Generated MCP config now includes the ai-rulez MCP server

### Fixed

- MCP tool configs now render with structured MCP output for supported tools

### Changed

- Bumped golangci-lint to v2.7.2 in Taskfile and CI

## 3.2.1 - 2025-12-28

### Fixed

- Gitignore updates now use relative paths instead of absolute machine-specific paths
- .ai-rulez directory is no longer added to .gitignore (source of truth should be tracked)
- Added tests to verify gitignore path handling

## 3.2.0 - 2025-12-28

### Added

- Full agent/subagent support in V3 configuration with `.ai-rulez/agents/` directory
- Auto-migration feature in generate command (detects V2 configs and migrates automatically)
- Interactive prompting for migration in terminal environments
- Silent auto-migration in CI environments
- `--auto-migrate` flag for explicit migration control
- Agent metadata support (name, description, model, tools, permission_mode, skills)
- Claude Code subagent format generation to `.claude/agents/`

### Fixed

- Migration mapping corrected: V2 sections → V3 skills, V2 agents → V3 agents
- Agent model field now preserved in YAML frontmatter during migration
- Backup directories automatically deleted on successful migration
- V3→V2 conversion now uses correct agent source
- Default profile now includes agents in generated content
- Code quality improvements (removed unused functions, reduced cyclomatic complexity)

### Changed

- Dependencies updated to latest minor versions (19 packages upgraded)
- Migration now creates proper directory structure for skills (skills/{id}/SKILL.md)

## 3.1.0 - 2025-12-27

### Added

- Migrate command for V2 to V3 configuration migration
- Comprehensive test suite for migrate command (27 tests covering command structure, flags, and utility functions)
- Integration test placeholder to prevent test runner errors

### Fixed

- Windows path separator issues in tests (content_test.go, validation_test.go, local_test.go)
- Windows absolute path generation for cross-platform test compatibility
- Test coverage for migrate command utilities (CopyDir, CreateBackup, detectV2Config)

## 3.0.0 - 2025-12-27

### Added

- Directory-based configuration system (`.ai-rulez/` directory structure)
- CRUD operations via CLI commands (domain, add, remove, list, include, profile)
- 22 MCP tools for AI assistant integration
- Domain separation for organizing rules by team/area
- Profile system for generating different configs for different teams
- Includes system for composing from local packages or Git repositories

### Changed

- **BREAKING**: Configuration format changed from single YAML to directory structure
- **BREAKING**: Init command no longer uses AI agents for dynamic initialization
- Documentation simplified and focused on V3 only

### Removed

- **BREAKING**: Enforce command removed
- **BREAKING**: V2 dynamic init with AI agents removed
- V2 single-file YAML configuration support

## 2.4.0 - 2025-10-22

### Fixed

- CLI MCP interference with template-generated files
- Output type values in AI-generated configurations

## 2.3.0 - 2025-10-05

### Added

- AI-powered rule enforcement system
- Automatic gitignore management
- Junie preset with lefthook configuration

## 2.2.0 - 2025-09-20

### Added

- MCP file-based configuration system for Claude and other tools

## 2.1.0 - 2025-08-20

### Added

- CLI MCP integration for Claude
- Improved init phase system

### Fixed

- Cross-platform binary extensions for Windows support
- Homebrew formula structure

## 2.0.0 - 2025-07-30

### Added

- Schema v2 with priority enum system
- Named target resolution for filter functions

### Changed

- **BREAKING**: Updated schema to v2 with priority enum system
- Unified section field naming to use 'name' instead of 'title'

## 1.6.0 - 2025-07-10

### Added

- Target filtering and named targets support

## 1.5.0 - 2025-06-25

### Added

- Initial release with core configuration generation
- Rule definition system
- Template-based generator
