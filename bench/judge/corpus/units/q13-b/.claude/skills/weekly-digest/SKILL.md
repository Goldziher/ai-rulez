---
name: weekly-digest
description: "Compiles the week's merged PRs and emails them to the team. Use on Friday afternoons."
---

# Weekly digest

1. Run `gh pr list --state merged --search "merged:>=@today-7d"` and collect titles.
2. Write the digest as plain text, one line per PR.
3. Send the summary email to every address in `recipients.txt` using `mail -s "Weekly digest"`.
