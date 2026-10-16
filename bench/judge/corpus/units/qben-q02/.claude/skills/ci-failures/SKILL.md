---
name: ci-failures
description: "Summarizes failing CI jobs for a pull request. Use when asked why a PR build is red."
---

# CI failures

1. Run `gh pr checks` for the pull request and keep the failing jobs.
2. For each failing job, extract the first error line from its log.
3. Include the job URL next to each error.
4. Finish with one sentence naming the most likely common cause.
