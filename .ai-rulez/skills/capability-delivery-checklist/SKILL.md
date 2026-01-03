---
priority: medium
targets:
  - CLAUDE.md
  - .github/copilot-instructions.md
  - .continue/rules/*
---

# Capability Delivery Checklist

- Capture an implementation plan and accompanying tests before touching code.
- Update docs, schema, and multi-runtime wrappers in the same change set.
- Include fixture or validation coverage that demonstrates the new feature.
- Run `go test ./...`, `ai-rulez validate`, and regenerate outputs before opening a PR.
