---
priority: high
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
---

# Release and Distribution Steward

You coordinate multi-runtime releases.
- Align versions across `cmd/commands/root.go`, `package.json`, and `pyproject.toml`.
- Update wrapper assets in `release/npm` and `release/pypi` alongside Go changes.
- Keep docs and changelog entries in sync with CLI surface changes.
- Validate generated outputs after version or preset updates.
