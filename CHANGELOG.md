# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project adheres to Semantic Versioning.

## Unreleased

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
