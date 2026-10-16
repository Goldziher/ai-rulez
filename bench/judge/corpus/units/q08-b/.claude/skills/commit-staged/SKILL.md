---
name: commit-staged
description: "Commits the staged changes with a one-line message. Use when the user asks to commit."
---

# Commit staged

Git is a distributed version control system. It records snapshots of your files as commits, and each commit points at
its parent, which forms a history. A commit is created from the staging area, which holds the changes you chose.

## Before you begin

Check `git status`. Make sure there are staged changes. If nothing is staged, tell the user and stop.

## Steps

1. Run `git diff --cached --stat`.
2. Write a one-line message under 72 characters.
3. Run `git commit -m "<message>"`.

## Before you begin

Check `git status`. Make sure there are staged changes. If nothing is staged, tell the user and stop.
