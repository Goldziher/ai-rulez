---
priority: high
---
# Go Conventions
- Follow Effective Go and Go Code Review Comments guidelines.
- Handle errors explicitly — check every error return. Wrap errors with context using `fmt.Errorf("context: %w", err)`.
- Use `golangci-lint` for linting. Format with `gofmt`.
- Prefer table-driven tests. Use `t.Parallel()` where safe.
- Keep packages small and focused. Avoid package-level state.
