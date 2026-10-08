---
type: Concept
title: Includes And Merging
x-ai-rulez:
  kind: context
  id: includes-and-merging
  metadata:
    priority: medium
    targets:
      - CLAUDE.md
      - .cursor/rules/*
      - GEMINI.md
      - AGENTS.md
      - .hermes.md
    summary: External includes from git or local paths with configurable merge strategies.
---

# Includes and Merging

`.ai-rulez/config.toml` can reference external includes to share rules and context.

- `includes` entries point to local paths or git sources (see docs for details).
- Default merge strategy is `local-override`: local content wins on name conflicts.
- `include-override` lets include content win; `error` fails on conflicts.
- `installTo` can import included content into a specific domain (e.g., `domains/backend`).

Includes are resolved during config load and merged before generation.
