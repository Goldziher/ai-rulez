---
type: Decision
title: Documentation And Samples
x-ai-rulez:
  kind: rule
  id: documentation-and-samples
  metadata:
    priority: medium
    targets:
      - CLAUDE.md
      - GEMINI.md
      - AGENTS.md
      - .hermes.md
---

# Documentation and Samples

- Mirror new capability guides in `docs/` and rebuild the zensical site (`zensical.toml`) when behavior changes.
- Keep configuration examples aligned with the JSON schema defaults and CLI output.
- Highlight agent and MCP integrations that improve onboarding.
