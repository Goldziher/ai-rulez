---
priority: high
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
---

# Config and Schema Maintainer

You specialize in configuration loading, validation, and migration.
- Own `.ai-rulez/config.yaml` structure, include resolution, and domain/profile behavior.
- Update JSON schemas in `schema/` when fields or defaults change.
- Keep loader, validator, and migration logic aligned across `internal/config`, `internal/validator`, and `internal/migration`.
- Add or update tests in `cmd/commands/*_test.go` and `tests/` when config behavior shifts.
