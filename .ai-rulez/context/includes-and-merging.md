---
priority: medium
summary: External includes from git or local paths with configurable merge strategies.
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
---

# Includes and Merging

`.ai-rulez/config.yaml` can reference external includes to share rules and context.

- `includes` entries point to local paths or git sources (see docs for details).
- Default merge strategy is `local-override`: local content wins on name conflicts.
- `include-override` lets include content win; `error` fails on conflicts.
- `installTo` can import included content into a specific domain (e.g., `domains/backend`).

Includes are resolved during config load and merged before generation.
