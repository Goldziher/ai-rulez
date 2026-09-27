---
name: code-quality-standards
description: "Concrete code-quality thresholds and anti-patterns: 120-char lines, max 20 cyclomatic complexity, max 4 nesting levels, max 50 lines per function, no magic numbers, no global state, composition over inheritance, no bare except, rule-of-three before extracting an abstraction, delete dead code instead of commenting it out. Load when writing a new function or module, reviewing a diff for quality, refactoring a long or deeply nested function, or deciding whether to extract shared logic."
---

## Readability

- Max 120 character line width.
- Prefer explicit code over clever tricks — if it needs a comment to explain what it does, rewrite it.
- No abbreviations in public API names (`context` not `ctx`, `repository` not `repo`).
- Keep functions short and focused on a single responsibility.

## Complexity limits

Max 20 cyclomatic complexity per function, max 4 levels of nesting depth, max 50 lines per function. Use early returns to flatten conditionals. Break complex functions into well-named helpers that each do one thing.

## Anti-patterns

- No magic numbers — use named constants.
- No global state — use dependency injection.
- No inheritance for code reuse — prefer composition.
- No bare exception handlers — catch specific types.
- No mocking internal services — use real objects for integration tests.
- No blocking I/O in async code paths — keep async paths fully async.

## Duplication

Extract shared logic after the third repetition, not before. Three similar lines are better than a premature abstraction. When extracting, ensure the shared code has a single reason to change — if two callers would evolve the logic differently, keep them separate. Premature abstraction creates worse coupling than duplication.

## Dead code

Remove dead code instead of commenting it out. Version control preserves history; commented-out code creates confusion and maintenance burden.
