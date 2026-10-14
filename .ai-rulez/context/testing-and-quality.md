---
type: Concept
title: Testing And Quality
x-ai-rulez:
  kind: context
  id: testing-and-quality
  metadata:
    priority: high
    targets:
      - CLAUDE.md
      - .cursor/rules/*
      - GEMINI.md
      - AGENTS.md
      - .hermes.md
    summary: Table-driven tests, fixtures, integration coverage, and deterministic testing practices.
---

## Testing and Quality

- Go unit tests use table-driven structure with Arrange/Act/Assert sections.
- `tests/` holds fixtures, integration coverage, e2e CLI tests, and platform scenarios.
- Generator, validation, and migration behavior is verified with fixture-driven tests.
- `tests/golden` records the output of every preset, so a new preset needs a golden; `tests/conformance` validates generated documents against vendored standard schemas; `tests/parity` keeps the CLI, docs and MCP tools in step; `tests/archlint` forbids ambient authority (working directory, environment, clock) in `internal/`.
- Prefer deterministic tests and avoid network or timing dependencies when possible.

Suggested commands:

- `go test ./...`
- `./bin/ai-rulez validate` (or `ai-rulez validate` if installed)
- Docs changes: `go run ./scripts/docsllms --check`, `go run ./scripts/clidocs --check`, `go test ./internal/llmstxt/... ./tests/...`, then `poly fmt --check .` and `poly lint .`.
