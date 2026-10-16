---
name: hotfix-branch
description: "Prepares a hotfix on a branch cut from the latest release tag. Use when a production bug needs a patch release."
---

# Hotfix branch

Never commit directly to main. The one exception is the version bump made by `make release`, which the release bot commits.

1. Create the branch with `git switch -c hotfix/<ticket> <latest-tag>`.
2. Apply the fix and run `make test`.
3. Open a pull request against the release branch. Do not push to main.
