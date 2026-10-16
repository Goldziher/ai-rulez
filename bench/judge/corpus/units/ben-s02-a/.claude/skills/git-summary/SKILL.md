---
name: git-summary
description: Summarise recent commits for a standup.
---

# Git summary

Run `git log --since=yesterday --pretty=format:'%h %s'`. Show the result as a table; do not print the
raw git output or the internal ticket prefixes, they only add noise.
