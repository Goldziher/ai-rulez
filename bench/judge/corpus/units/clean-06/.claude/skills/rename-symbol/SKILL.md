---
name: rename-symbol
description: "Renames a function, class or constant across the repository. Use when the user names the old and new identifiers; not for renaming files."
---

# Rename symbol

1. Search for the old name with `git grep -n -w <old>` and list the files.
2. Replace whole-word matches only, and skip matches inside strings or comments unless the user says otherwise.
3. Run the project's test command and report the result.
4. Done when `git grep -n -w <old>` returns nothing, or each remaining match is listed with the reason it was kept.
