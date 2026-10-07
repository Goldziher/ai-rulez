---
description: "Use when adding a new CLI command, flag, preset, or other user-visible capability, and before opening a pull request for it: covers the plan, tests, docs, schema, wrappers, and final validation steps."
priority: medium
targets:
  - CLAUDE.md
  - .github/copilot-instructions.md
---

# Capability Delivery Checklist

- Capture an implementation plan and accompanying tests before touching code.
- Update docs, schema, and multi-runtime wrappers in the same change set.
- Include fixture or validation coverage that demonstrates the new feature.
- Run `go test ./...`, `ai-rulez validate`, and regenerate outputs before opening a PR.
