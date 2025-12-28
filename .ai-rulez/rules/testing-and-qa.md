---
priority: high
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - .github/copilot-instructions.md
---

# Testing and QA

- Prefer table-driven Go tests with clear Arrange/Act/Assert phases.
- Cover failure paths around file IO, CLI flags, concurrency, and external integrations.
- Use fixtures under `tests/` to exercise generator and enforcement workflows instead of brittle snapshots.
- Run `ai-rulez validate` and `ai-rulez enforce` in CI to keep configuration healthy.
