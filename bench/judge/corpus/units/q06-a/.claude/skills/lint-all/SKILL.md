---
name: lint-all
description: "Runs formatting and lint checks with acme-tool. Use before opening a pull request."
---

# Lint all

The `fmt` subcommand of `acme-tool` was removed in v3; use `acme-tool format` instead.

1. Run `acme-tool lint`.
2. Run `acme-tool fmt --check` to verify formatting.
3. Report the files that failed either step.
