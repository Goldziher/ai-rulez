# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project adheres to Semantic Versioning.

## Unreleased

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
