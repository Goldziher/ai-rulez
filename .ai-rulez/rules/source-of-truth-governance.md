---
type: Decision
title: Source Of Truth Governance
description: Apply when working with AI tooling configuration
x-ai-rulez:
  kind: rule
  id: source-of-truth-governance
  metadata:
    priority: critical
    targets:
      - CLAUDE.md
      - .cursor/rules/*
      - .devin/*
      - .github/copilot-instructions.md
      - AGENTS.md
      - .hermes.md
    trigger: model_decision
---

# Source-of-Truth Governance

- Treat `.ai-rulez/config.toml` and the `.ai-rulez/` content tree as the canonical configuration for all AI tooling.
- Modify source files first, then regenerate assistant outputs with `ai-rulez generate`.
- YAML and JSON config files (`ai-rulez.yaml`, `config.yaml`) are not read by v5; `ai-rulez migrate v5` converts them.
- Reject manual edits to generated tool files; ensure diffs show regenerated content only.
- Keep templates and tests aligned so regenerated files remain stable and consistently ordered.
