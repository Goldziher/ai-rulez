---
description: "Adds a unit test for one function."
argument-hint: "<function-name>"
---

# Add test

1. Locate `$ARGUMENTS` with `git grep -n "def $ARGUMENTS"` and read it.
2. Write one test for the normal case and one for the main edge case next to the existing tests.
3. Run the new tests.

Done when both tests pass. If one fails because of a real bug, report the bug and do not change the function.
