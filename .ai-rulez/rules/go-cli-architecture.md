---
priority: critical
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - .github/copilot-instructions.md
  - .windsurf/*
---

# Go CLI Architecture

- Keep Cobra commands under `cmd/commands` and register them in `cmd/commands/root.go`.
- Move reusable logic into the appropriate package under `internal/` (config, generator, enforcement, MCP, CRUD, etc.).
- Wrap errors with the `oops` helpers and route user-facing logs through `internal/logger`.
- Provide table-driven tests beside the command or package you touch.
