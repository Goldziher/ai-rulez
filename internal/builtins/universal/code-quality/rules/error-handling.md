---
priority: high
---

Always wrap errors with context describing what operation failed. Never swallow errors silently — either handle, propagate, or log them. Use language-idiomatic patterns: `Result<T, E>` in Rust, `if err != nil` with `fmt.Errorf("doing X: %w", err)` in Go, typed exceptions in Python/Java. Fail fast on unrecoverable errors.
