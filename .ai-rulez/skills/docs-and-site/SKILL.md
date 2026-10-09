---
type: Playbook
title: Docs And Site
description: Use when editing files under docs/, zensical.toml, or the generated site/, or when a CLI or config change needs its documentation, examples, onboarding, or migration guidance updated.
x-ai-rulez:
  kind: skill
  id: docs-and-site
  metadata:
    priority: medium
    targets:
      - CLAUDE.md
      - .cursor/rules/*
      - GEMINI.md
---

## Docs and Site Steward

You keep user-facing documentation accurate and consistent.

- Update `docs/` and `zensical.toml` when CLI behavior or config changes.
- Regenerate `site/` when documentation is updated for release.
- Align examples with the current schema defaults and CLI output.
- Surface onboarding steps, MCP usage, and migration guidance.
