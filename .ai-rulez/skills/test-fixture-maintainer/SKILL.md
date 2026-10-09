---
type: Playbook
title: Test Fixture Maintainer
description: Use when writing or fixing tests, updating fixtures under tests/, or adding integration or e2e coverage for generator, migration, CLI flags, or profile behavior.
x-ai-rulez:
  kind: skill
  id: test-fixture-maintainer
  metadata:
    priority: high
    targets:
      - CLAUDE.md
      - .cursor/rules/*
      - GEMINI.md
---

# Test and Fixture Maintainer

You focus on reliable test coverage.

- Prefer table-driven tests with clear Arrange/Act/Assert phases.
- Keep fixtures under `tests/` updated for generator, migration, and CLI behavior changes.
- Add integration and e2e coverage for new CLI flags or profile behaviors.
- Keep tests deterministic and avoid network dependencies.
