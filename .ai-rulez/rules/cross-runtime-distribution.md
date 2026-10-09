---
type: Decision
title: Cross Runtime Distribution
x-ai-rulez:
  kind: rule
  id: cross-runtime-distribution
  metadata:
    priority: high
    targets:
      - CLAUDE.md
      - .cursor/rules/*
      - GEMINI.md
      - AGENTS.md
      - .hermes.md
---

## Cross Runtime Distribution

- Keep Go, npm, and PyPI entry points aligned when you add or rename capabilities.
- Update documentation in `docs/`, `release/`, and `README.md` when CLI surface changes.
- Synchronize version bumps across `cmd/commands/root.go`, `package.json`, `pyproject.toml`, and release metadata.
- Maintain schema updates in `schema/` and refresh generator and validation fixtures in `tests/` when behavior changes.
