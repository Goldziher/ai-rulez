---
name: code-quality-standards
description: "Code-quality limits: 120-char lines, complexity 20, nesting 4, 50-line functions, no magic numbers or global state, rule of three, delete dead code, narrow suppressions, tracked TODOs. Use when writing, reviewing or refactoring code."
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

## Suppressions

Fix the cause instead of suppressing a lint rule, type error or test. A necessary suppression (lint-disable, noqa, ts-ignore, `#[allow]`) is scoped to the narrowest line, names the rule, and gives a one-line reason. Never update a snapshot or baseline just to hide a failure.

## Markers

`TODO`, `FIXME`, `HACK` and `XXX` take the form `TODO(PROJ-123): description`. The key is an existing open tracker issue for future work, never the issue of the current change (it closes on merge). No bare markers, owners or dates.
