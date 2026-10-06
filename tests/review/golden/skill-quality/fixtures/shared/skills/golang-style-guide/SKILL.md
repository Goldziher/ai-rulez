---
name: golang-style-guide
description: "Go style rules: error wrapping, linting and table-driven tests. Load when writing or reviewing Go code."
---
- Wrap errors with context using fmt.Errorf and %w.
- Run golangci-lint with a strict config and format with gofmt.
- Write table-driven tests with t.Run.
- Accept interfaces, return structs.
