---
priority: critical
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - .windsurf/*
  - .github/copilot-instructions.md
trigger: model_decision
description: Apply when working with AI tooling configuration

---

# Source-of-Truth Governance

- Treat `.ai-rulez/config.yaml` and the `.ai-rulez/` content tree as the canonical configuration for all AI tooling.
- Modify source files first, then regenerate assistant outputs with `ai-rulez generate`.
- Use `ai-rulez.yaml` only as a V2 migration input (`ai-rulez migrate v3`).
- Reject manual edits to generated tool files; ensure diffs show regenerated content only.
- Keep templates and tests aligned so regenerated files remain stable and consistently ordered.
