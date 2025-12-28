---
priority: high
targets:
  - CLAUDE.md
  - .cursor/rules/*
---

# Repository Layout

## Core Go CLI
- `cmd/main.go`: boots the CLI and injects the version string.
- `cmd/commands/`: Cobra command implementations (`generate`, `enforce`, CRUD helpers).
- `internal/`: service packages for config loading, generator, enforcement, MCP, templates, and utilities.

## Multi-runtime Wrappers
- `release/npm` and `release/pypi`: JavaScript and Python entry points backed by the Go binary.
- `homebrew-tap`: tap formula for Homebrew distribution.

## Documentation & Tooling
- `docs/`, `site/`, and `mkdocs.yaml`: user-facing documentation and site generation.
- `tests/` and `testing/`: fixtures, integration scenarios, and validation helpers.
