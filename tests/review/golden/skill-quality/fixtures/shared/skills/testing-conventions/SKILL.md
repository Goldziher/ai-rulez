---
name: testing-conventions
description: "How to write a good test: behaviour-describing names (`should_return_error_when_input_is_empty`, `given_when_then`), exact-value assertions over truthiness, snapshot and property-based testing, order-independent and idempotent tests with no shared mutable state, and the anti-patterns (testing mocks, test-only production methods, mocking what you don't own, asserting implementation details). Load when writing or reviewing a test file, naming test cases, choosing assertions, or diagnosing a flaky or always-passing test."
---

## Naming

Name tests to describe behaviour: `should_return_error_when_input_is_empty`, `test_parse_handles_nested_objects`. Use `describe`/`it` blocks for grouping where the language supports them. Follow `given_when_then` or `should_when`. Test names are specifications — a reader should understand the expected behaviour without reading the body.

## Assertions

Assert exact expected values, not truthiness (`assert result == 42`, not `assert result`). Use snapshot testing for complex structured output and property-based testing for functions with wide input ranges. Include descriptive failure messages. Always test error paths and edge cases, not just the happy path.

## Independence

Tests must be independent and idempotent — runnable in any order, in parallel. No shared mutable state between tests. Use factories or fixtures for setup. Clean up created resources (files, DB rows, env vars) after each test. Never rely on execution order.

## Anti-patterns

- Do not test mock behaviour instead of real behaviour.
- Do not add test-only methods to production code.
- Do not mock what you don't own — wrap it and test the wrapper.
- Do not test implementation details — test observable behaviour.
- Do not write tests that pass when the code is broken. If a test never fails, it is not testing anything.
