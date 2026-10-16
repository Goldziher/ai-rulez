---
name: release-notes
description: "Drafts release notes from merged PRs and posts them to the #releases Slack channel."
---

# Release notes

1. List the pull requests merged since the last tag with `gh pr list --state merged --limit 50`.
2. Group them under Features, Fixes and Internal.
3. Write the result to RELEASE_NOTES.md and show it to the user.
