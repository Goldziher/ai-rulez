---
priority: critical
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - .windsurf/*
  - .github/copilot-instructions.md
---

# Source-of-Truth Governance

- Treat `ai-rulez.yaml` as the canonical configuration for all AI tooling.
- Modify this file first, then regenerate assistant outputs with `ai-rulez generate`.
- Reject manual edits to generated tool files; ensure diffs show regenerated content only.
- Keep templates and tests aligned so regenerated files remain stable and consistently ordered.
