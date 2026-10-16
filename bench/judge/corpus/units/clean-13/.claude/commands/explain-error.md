---
description: "Explains an error found in a log file and proposes a next step."
argument-hint: "<log-file>"
---

# Explain error

1. Read the last 200 lines of `$ARGUMENTS`.
2. Find the first line containing `ERROR` or `Traceback`.
3. Explain the error in two sentences and name the most likely cause.
4. Propose one next step the user can run.

Done when the explanation and the next step are written. If the file has no error line, say so and stop.
