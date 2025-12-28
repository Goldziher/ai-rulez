---
priority: high
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
  - .continue/**/*.yaml
---

# Cross Runtime Distribution

- Keep Go, npm, and PyPI entry points aligned when you add or rename capabilities.
- Update documentation in `docs/`, `release/`, and `README.md` when CLI surface changes.
- Synchronize version bumps across `cmd/commands/root.go`, `package.json`, `pyproject.toml`, and release metadata.
- Maintain schema updates in `schema/` and refresh generator/enforcer fixtures in `tests/` when behavior changes.
