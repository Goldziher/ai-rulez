---
description: "Use when you are unsure where code, docs, schemas, wrappers, or tests live in this repository, or need to pick the right directory before adding or moving files."
priority: high
targets:
  - CLAUDE.md
  - .cursor/rules/*
---

# Repository Layout

## Core Go CLI

- `cmd/ai-rulez/main.go`: boots the CLI and injects the version string.
- `cmd/commands/`: Cobra command implementations (`generate`, `validate`, `mcp`, CRUD helpers).
- `internal/`: service packages for config loading, generator, templates, includes, CRUD, MCP, validator, logger, and migration.

## Multi-runtime Wrappers

- `release/npm` and `release/pypi`: JavaScript and Python entry points backed by the Go binary.
- `package.json` and `pyproject.toml`: publish metadata for npm and PyPI wrappers.
- Homebrew distribution is managed via a tap outside this repository.

## Documentation & Tooling

- `docs/`, `site/`, and `zensical.toml`: user-facing documentation and site generation.
- `schema/`: JSON schemas for configuration and MCP files.
- `tests/`: fixtures, integration, e2e, and platform coverage.
