---
priority: high
---
Write tests before writing code, update tests when modifying behavior. When fixing bugs, write a failing test first — RED (failing test) → GREEN (minimal code to pass) → REFACTOR. Wrote production code before the test? Delete it, start over — no exceptions, don't keep as reference. Integration tests for API surfaces, unit tests for business logic, property tests for edge-case-heavy code. Run the full test suite before committing — never push untested code.
