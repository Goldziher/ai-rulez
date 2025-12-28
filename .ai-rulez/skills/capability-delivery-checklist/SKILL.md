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
- Include enforcement coverage or fixtures that demonstrate the new feature.
- Run `go test ./...` and regenerate outputs before opening a PR.
