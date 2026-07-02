---
priority: high
---

# poly

poly (polylint) is a single-binary, multi-language linter and formatter. It bundles engines (ruff, oxc, taplo, rumdl) and delegates to native tools (cargo fmt/clippy, golangci-lint, actionlint, shellcheck, shfmt) when present.

## Commands

- Lint: `poly lint .`
- Check formatting (dry-run): `poly fmt --check .`
- Apply formatting: `poly fmt --fix .`

## Configuration

Per-repo `poly.toml`. Cache dir `.polylint/` (gitignored).

## Severity

`poly lint` exits non-zero only on error-severity findings; warnings don't fail CI.

Run `poly fmt --check .` and `poly lint .` after changes to verify compliance.
