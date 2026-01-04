---
priority: high
summary: Table-driven tests, fixtures, integration coverage, and deterministic testing practices.
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
---

# Testing and Quality

- Go unit tests use table-driven structure with Arrange/Act/Assert sections.
- `tests/` holds fixtures, integration coverage, e2e CLI tests, and platform scenarios.
- Generator, validation, and migration behavior is verified with fixture-driven tests.
- Prefer deterministic tests and avoid network or timing dependencies when possible.

Suggested commands:
- `go test ./...`
- `./bin/ai-rulez validate` (or `ai-rulez validate` if installed)
