---
name: testing-conventions
description: "Writing tests: behavior-describing names, exact-value assertions, snapshot and property tests, independent idempotent tests, anti-patterns such as testing mocks. Use when writing or reviewing tests or diagnosing a flaky one."
---

## Naming

Name tests to describe behaviour: `should_return_error_when_input_is_empty`, `test_parse_handles_nested_objects`. Use `describe`/`it` blocks for grouping where the language supports them. Follow `given_when_then` or `should_when`. Test names are specifications — a reader should understand the expected behaviour without reading the body.

## Assertions

Assert exact expected values, not truthiness (`assert result == 42`, not `assert result`). Use snapshot testing for complex structured output and property-based testing for functions with wide input ranges. Include descriptive failure messages. Always test error paths and edge cases, not just the happy path. Use parameterized tests or fixtures for input variations instead of copy-paste. Do not test that an exception class or function merely exists.

## Independence

Tests must be independent and idempotent — runnable in any order, in parallel. No shared mutable state between tests. Use factories or fixtures for setup. Clean up created resources (files, DB rows, env vars) after each test. Never rely on execution order. Mock only the external boundary (network, third-party services, clocks); use real objects for everything else.

## Anti-patterns

- Do not test mock behaviour instead of real behaviour.
- Do not add test-only methods to production code.
- Do not mock what you don't own — wrap it and test the wrapper.
- Do not test implementation details — test observable behaviour.
- Do not write tests that pass when the code is broken. If a test never fails, it is not testing anything.
- When adding tests to existing code, never edit the code under test to make a test pass. If a test can only pass that way, it found a bug: surface it. In a red-green cycle the failing test comes first and the code change is the point. Fixing the test's own scaffolding, mocks or assertions is always in scope.
- Do not skip, xfail or weaken a failing test to get green. A skipped test or vacuous pass hides a gap.

## Verify the test can fail

After a test passes, break or revert the fix and confirm it fails for the reason you expect. A test file the runner does not collect, or an empty fixture, passes while testing nothing: check that the test actually ran.
