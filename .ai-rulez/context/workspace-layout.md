---
type: Concept
title: Workspace Layout
x-ai-rulez:
  kind: context
  id: workspace-layout
  metadata:
    priority: high
    targets:
      - CLAUDE.md
      - .cursor/rules/*
      - GEMINI.md
      - AGENTS.md
      - .hermes.md
    summary: Repository structure covering CLI, wrappers, documentation, schemas, and test organization.
---

## Workspace Layout

Key directories:

- `cmd/ai-rulez/main.go` boots the CLI; `cmd/commands/` holds Cobra commands.
- `internal/` contains service packages for config loading, generation, templates, includes, CRUD, MCP, validation, logging, and migration.
- `schema/` holds JSON schemas for config and MCP files.
- `docs/`, `zensical.toml`, and `site/` contain documentation sources and generated site output.
- `release/npm` and `release/pypi` hold JavaScript and Python wrappers around the Go binary.
- `tests/` contains fixtures, integration, e2e CLI coverage, and platform tests.

Convenience:

- `bin/ai-rulez` is a local build used for running `generate`, `validate`, and other commands in development.
