---
name: start-ticket
description: "Creates the git branch for a ticket. Use when the user starts a ticket and gives its key and a short slug."
---

# Start ticket

1. Run `scripts/branch-name.sh <ticket> <slug>` from this skill's directory; it prints the branch name.
2. Create the branch with `git switch -c "$(scripts/branch-name.sh <ticket> <slug>)"`.
3. Show the user the new branch name. The task is done when `git branch --show-current` prints it.
