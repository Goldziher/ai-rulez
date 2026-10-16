---
description: "Creates a numbered architecture decision record from a title."
argument-hint: "<title>"
---

# New ADR

1. List the existing files in `docs/adr/` and take the highest number plus one, zero-padded to four digits.
2. Create `docs/adr/<number>-<kebab-title>.md` for the title `$ARGUMENTS`, with the headings Status, Context, Decision and Consequences.
3. Set Status to `Proposed` and leave the other sections for the user.

Done when the new file exists and its path has been shown to the user.
