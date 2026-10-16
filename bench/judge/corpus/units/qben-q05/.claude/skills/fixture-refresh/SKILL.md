---
name: fixture-refresh
description: "Refreshes test fixtures from recorded API responses. Use when an upstream response format changed."
---

# Fixture refresh

A fixture pack is a directory under `testdata/` that holds `input.json` and `expected.json`.

1. Pick the fixture pack the user names.
2. Replace `input.json` with the recorded response.
3. Run `make test` and rewrite `expected.json` only for failures that come from the format change.
4. Report each changed pack.
