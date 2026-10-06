---
name: tdd-workflow
description: "Test-driven development discipline: RED (failing test), GREEN (minimal code), REFACTOR. Load before starting a feature or bug fix, or when reproducing a reported bug. Not for naming tests or choosing assertions (see testing-conventions)."
---

- Write the test before the code. Update the test when changing behaviour.
- Fixing a bug: write the failing test first, then fix. RED → GREEN → REFACTOR.
- Wrote production code before the test? Delete it and start over — no exceptions, don't keep it as reference.
- Integration tests for API surfaces, unit tests for business logic, property tests for edge-case-heavy code.
- Run the full test suite before committing — never push untested code.
