# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [3.2.1] - 2025-12-28

### Fixed
- Gitignore updates now use relative paths instead of absolute machine-specific paths
- .ai-rulez directory is no longer added to .gitignore (source of truth should be tracked)
- Added tests to verify gitignore path handling

## [3.2.0] - 2025-12-28

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

## [3.1.0] - 2025-12-27

### Added
- Migrate command for V2 to V3 configuration migration
- Comprehensive test suite for migrate command (27 tests covering command structure, flags, and utility functions)
- Integration test placeholder to prevent test runner errors

### Fixed
- Windows path separator issues in tests (content_test.go, validation_test.go, local_test.go)
- Windows absolute path generation for cross-platform test compatibility
- Test coverage for migrate command utilities (CopyDir, CreateBackup, detectV2Config)

## [3.0.0] - 2025-12-27

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

## [2.4.0] - 2025-10-22

### Fixed
- CLI MCP interference with template-generated files
- Output type values in AI-generated configurations

## [2.3.0] - 2025-10-05

### Added
- AI-powered rule enforcement system
- Automatic gitignore management
- Junie preset with lefthook configuration

## [2.2.0] - 2025-09-20

### Added
- MCP file-based configuration system for Claude and other tools

## [2.1.0] - 2025-08-20

### Added
- CLI MCP integration for Claude
- Improved init phase system

### Fixed
- Cross-platform binary extensions for Windows support
- Homebrew formula structure

## [2.0.0] - 2025-07-30

### Added
- Schema v2 with priority enum system
- Named target resolution for filter functions

### Changed
- **BREAKING**: Updated schema to v2 with priority enum system
- Unified section field naming to use 'name' instead of 'title'

## [1.6.0] - 2025-07-10

### Added
- Target filtering and named targets support

## [1.5.0] - 2025-06-25

### Added
- Initial release with core configuration generation
- Rule definition system
- Template-based generator

---

[Unreleased]: https://github.com/Goldziher/ai-rulez/compare/v3.2.0...HEAD
[3.2.0]: https://github.com/Goldziher/ai-rulez/releases/tag/v3.2.0
[3.1.0]: https://github.com/Goldziher/ai-rulez/releases/tag/v3.1.0
[3.0.0]: https://github.com/Goldziher/ai-rulez/releases/tag/v3.0.0
[2.4.0]: https://github.com/Goldziher/ai-rulez/releases/tag/v2.4.0
[2.3.0]: https://github.com/Goldziher/ai-rulez/releases/tag/v2.3.0
[2.2.0]: https://github.com/Goldziher/ai-rulez/releases/tag/v2.2.0
[2.1.0]: https://github.com/Goldziher/ai-rulez/releases/tag/v2.1.0
[2.0.0]: https://github.com/Goldziher/ai-rulez/releases/tag/v2.0.0
[1.6.0]: https://github.com/Goldziher/ai-rulez/releases/tag/v1.6.0
[1.5.0]: https://github.com/Goldziher/ai-rulez/releases/tag/v1.5.0
