---
description: "Runs the linter on the working tree and reports failures."
argument-hint: "<path>"
---

# Check

1. Run `make lint PATH_ARG=$ARGUMENTS`.
2. If it fails, report the first failing file with its message and stop.

Done when `make lint` exits 0, or when you have reported the first failure.
