# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.6.0] - 2025-08-20

### Added
- **Target Filtering**: Rules, sections, and agents can now specify target patterns to control which outputs they appear in
  - Support for glob patterns (e.g., `*.md`, `docs/**/*.md`) 
  - File extension matching (e.g., `.cursorrules`)
  - Directory-specific targeting (e.g., `src/frontend/**`)
- **Named Targets**: Define reusable target groups that can be referenced with `@target-name` syntax
  - Configure named targets in config metadata: `targets: { "claude-files": ["CLAUDE.md", ".claude/**/*.md"] }`
  - Reference in rules/sections/agents: `targets: ["@claude-files"]`
  - Mix named targets with inline patterns: `targets: ["@claude-files", "*.md"]`

### Changed
- Enhanced configuration schema to support target filtering capabilities
- Improved template engine to handle target-aware filtering of rules, sections, and agents
- Updated CLI commands to support target management operations

### Technical Details
- Added `ResolveTargets()` function for expanding named target references
- Added `FilterRules()`, `FilterSections()`, and `FilterAgents()` functions for output-specific filtering
- Enhanced performance with optimized pattern matching and concurrent processing
- Comprehensive test coverage for all target filtering scenarios

## [1.5.1] - 2025-01-09

### Fixed
- Release pipeline improvements
- Dependencies updated for security patches

## [1.5.0] - Previous Release

### Added
- MCP (Model Context Protocol) server functionality
- Enhanced CLI commands for comprehensive CRUD operations
- Remote configuration includes with security validation
- Advanced caching mechanisms for performance optimization

## Earlier Versions

See Git history for detailed changes in versions 1.0.0 through 1.4.3.