---
type: Playbook
title: Repository Layout
description: Use when you are unsure where code, docs, schemas, wrappers, or tests live in this repository, or need to pick the right directory before adding or moving files.
x-ai-rulez:
  kind: skill
  id: repository-layout
  metadata:
    priority: high
    targets:
      - CLAUDE.md
      - .cursor/rules/*
---

## Repository Layout

### Core Go CLI

- `cmd/ai-rulez/main.go`: boots the CLI and injects the version string.
- `cmd/commands/`: Cobra command implementations (`generate`, `validate`, `mcp`, CRUD helpers).
- `internal/`: service packages for config loading, generator, templates, includes, CRUD, MCP, validator, logger, and migration.

### Multi-runtime Wrappers

- `release/npm` and `release/pypi`: JavaScript and Python entry points backed by the Go binary.
- `release/npm/package.json` and `release/pypi/setup.py`: publish metadata for the npm and PyPI wrappers.
- Homebrew distribution is managed via a tap outside this repository.

### Documentation & Tooling

- `website/`: the documentation site (Astro Starlight); `docs/`: maintainer guides and README media.
- `schema/`: JSON schemas for configuration and MCP files.
- `tests/`: fixtures, integration, e2e, and platform coverage.
