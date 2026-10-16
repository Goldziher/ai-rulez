---
name: dependency-bump
description: "Bumps one dependency to a newer version. Use when the user names a package to upgrade."
---

# Dependency bump

## Steps

1. Edit the version of the named package in the manifest.
2. Run `make lock` to refresh the lockfile.
3. Run `make test` and report the result.

## Not for

Major-version upgrades that need code changes; plan those with the user first.

Owner: Platform team
