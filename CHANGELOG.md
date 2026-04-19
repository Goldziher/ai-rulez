# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project adheres to Semantic Versioning.

## Unreleased

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
