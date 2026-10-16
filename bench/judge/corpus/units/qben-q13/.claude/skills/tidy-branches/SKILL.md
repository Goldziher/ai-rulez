---
name: tidy-branches
description: "Cleans up stale git branches. Use when the repository has many merged branches."
---

# Tidy branches

1. List remote branches already merged into main.
2. Show the list to the user and ask which ones to delete. Do not delete anything before they answer.
3. Delete the confirmed branches with `git push origin --delete <branch>`.
4. Report which branches were deleted.
