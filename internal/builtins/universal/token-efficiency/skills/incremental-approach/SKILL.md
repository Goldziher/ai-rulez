---
name: incremental-approach
description: "Build in verified increments: smallest change first, verify, then extend; prototype the risky part first; batch edits to one file. Use when planning a large or multi-file change or choosing between approaches."
---

- Start with the smallest viable change, verify it works, then extend. Implement one piece, test, move on.
- Avoid generating large blocks of speculative code.
- When uncertain about an approach, prototype the critical part before committing to the full implementation.
- Batch multiple edits to the same file into one operation, and prefer multi-file search over reading files one by one when exploring.
