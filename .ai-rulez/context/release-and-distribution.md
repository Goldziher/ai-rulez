---
priority: high
summary: Multi-channel distribution (Go, npm, PyPI, Homebrew) with aligned versioning.
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
---

# Release and Distribution

AI-Rulez ships through multiple channels:
- Go binary (main CLI entry point).
- npm wrapper under `release/npm`.
- PyPI wrapper under `release/pypi`.
- Homebrew distribution is handled via tap metadata outside this repository.

Versioning and metadata:
- Keep versions aligned in `cmd/commands/root.go`, `package.json`, and `pyproject.toml`.
- Update release notes and docs when CLI surface changes.
- Regenerate outputs after changing `.ai-rulez/` content.
