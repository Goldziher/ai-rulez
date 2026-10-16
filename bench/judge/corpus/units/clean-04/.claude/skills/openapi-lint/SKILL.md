---
name: openapi-lint
description: "Lints an OpenAPI file with the bundled Spectral wrapper. Use when the user edits an OpenAPI spec and wants it validated."
---

# OpenAPI lint

1. Run `scripts/lint.sh <spec-file>` from this skill's directory.
2. Group the reported problems by severity.
3. Done when the script exits 0 or every reported problem is listed with its line number.
