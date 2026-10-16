---
name: lockfile-refresh
description: "Regenerates the dependency lockfile. Use when the manifest changed and the lockfile is out of date."
---

# Lockfile refresh

## Steps

1. Run `make lock`.
2. Show `git diff --stat` for the lockfile.

## Verify

Run `make lock-check` and confirm that it exits 0.

## Not for

Upgrading dependency versions; use `dependency-bump` for that.

Owner: Platform team
