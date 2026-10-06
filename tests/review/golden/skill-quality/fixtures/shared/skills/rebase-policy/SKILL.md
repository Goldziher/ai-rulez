---
name: rebase-policy
description: "Keep a feature branch current with main. Use when asked how to update or sync a branch; not for merging it into main."
---
- Always rebase the feature branch onto main to pick up new work.
- Never create merge commits when syncing with main.
- Force-push the rebased branch with --force-with-lease.
