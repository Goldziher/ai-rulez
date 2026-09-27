---
name: tdd-workflow
description: "Test-driven development discipline: RED (failing test) → GREEN (minimal code) → REFACTOR, delete-and-restart if production code was written first, and the test-type taxonomy (integration for API surfaces, unit for business logic, property for edge-case-heavy code). Load before starting a feature or bug fix, when reproducing a reported bug, or when deciding which kind of test a change needs."
---

- Write the test before the code. Update the test when changing behaviour.
- Fixing a bug: write the failing test first, then fix. RED → GREEN → REFACTOR.
- Wrote production code before the test? Delete it and start over — no exceptions, don't keep it as reference.
- Integration tests for API surfaces, unit tests for business logic, property tests for edge-case-heavy code.
- Run the full test suite before committing — never push untested code.
