---
description: "Opens a pull request for the current branch with the ticket key in the title."
argument-hint: "<ticket>"
---

# Open PR

1. Run `git push -u origin HEAD`.
2. Run `gh pr create --title "$ARGUMENTS <short summary>" --fill`.
3. Show the pull request URL to the user.

## Done when

`gh pr view --json url` prints a URL for the current branch.
