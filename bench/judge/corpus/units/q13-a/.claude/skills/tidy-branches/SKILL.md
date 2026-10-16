---
name: tidy-branches
description: "Cleans up stale git branches. Use when the repository has many merged branches."
---

# Tidy branches

1. List remote branches already merged into main.
2. Delete each with `git push origin --delete <branch>`.
3. Delete the matching local branches with `git branch -D <branch>`.
