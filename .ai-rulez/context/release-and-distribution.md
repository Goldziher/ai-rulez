---
type: Concept
title: Release And Distribution
x-ai-rulez:
  kind: context
  id: release-and-distribution
  metadata:
    priority: high
    targets:
      - CLAUDE.md
      - .cursor/rules/*
      - GEMINI.md
      - AGENTS.md
      - .hermes.md
    summary: Multi-channel distribution (Go, npm, PyPI, Homebrew) with aligned versioning.
---

## Release and Distribution

AI-Rulez ships through multiple channels:

- Go binary (main CLI entry point).
- npm wrapper under `release/npm`.
- PyPI wrapper under `release/pypi`.
- Homebrew distribution is handled via tap metadata outside this repository.
- `action.yml` at the repository root is a composite GitHub Action that installs a sha256-verified release binary.

Versioning and metadata:

- Keep versions aligned in `cmd/commands/root.go`, `package.json`, and `pyproject.toml`.
- `task set-version -- X.Y.Z` bumps every manifest; `task docs:llms` regenerates `docs/llms.txt` and `docs/llms-full.txt`.
- Update release notes and docs when CLI surface changes.
- Regenerate outputs after changing `.ai-rulez/` content.
