---
priority: critical
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - .devin/*
  - .github/copilot-instructions.md
  - AGENTS.md
  - .hermes.md
trigger: model_decision
description: Apply when working with AI tooling configuration
---

# Source-of-Truth Governance

- Treat `.ai-rulez/config.toml` and the `.ai-rulez/` content tree as the canonical configuration for all AI tooling.
- Modify source files first, then regenerate assistant outputs with `ai-rulez generate`.
- V2/V3 config files (`ai-rulez.yaml`, `config.yaml`) are not read by v5; they are migrated with ai-rulez 4.x before upgrading.
- Reject manual edits to generated tool files; ensure diffs show regenerated content only.
- Keep templates and tests aligned so regenerated files remain stable and consistently ordered.
