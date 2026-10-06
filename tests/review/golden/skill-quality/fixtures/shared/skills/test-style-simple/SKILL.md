---
name: test-style-simple
description: "Style for small Go unit tests. Use when writing Go tests with one or two cases; not for large suites."
---
- Never use table-driven tests; write one test function per case.
- Name tests Test<Thing>_<Behaviour>.
- Keep each test under twenty lines.
