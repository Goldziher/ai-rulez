---
description: "Runs the formatter and linter on one path and applies safe fixes."
argument-hint: "<path>"
---

# Lint fix

1. Run `ruff check --fix $ARGUMENTS`.
2. Run `ruff format $ARGUMENTS`.
3. Run `git diff --stat` and list the changed files.

Done when both tools exit 0. If either fails, show its last 20 lines of output and stop.
