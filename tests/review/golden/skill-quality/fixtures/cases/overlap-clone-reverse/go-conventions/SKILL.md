---
name: go-conventions
description: Go code conventions covering Effective Go, error wrapping, golangci-lint, govulncheck, table-driven tests, interfaces, context usage, modules, and concurrency. Load when writing or reviewing Go code.
---

- Follow Effective Go and Go Code Review Comments guidelines.
- Handle every error return. Wrap errors with context: `fmt.Errorf("operation failed: %w", err)`.
- Linting: a meta-linter (e.g., golangci-lint) with strict config (enable `govet`, `staticcheck`, `errcheck`, and a security analyzer). Format with `gofmt`/`goimports`.
- Security: `govulncheck` for CVE scanning, a SAST tool for static analysis. Run both in CI.
- Testing: table-driven tests with `t.Run()` and the stdlib `testing` package. Use `t.Parallel()` where safe. An assertion library (e.g., testify) is optional. Coverage with `go test -coverprofile`.
- Naming: use short, descriptive names. Receivers are 1-2 letters. Exported names are descriptive.
- Prefer composition over inheritance. Use interfaces for abstraction (accept interfaces, return structs).
- Keep packages small and focused. Avoid package-level state and `init()` functions.
- Use `context.Context` as first parameter for cancelable operations. Never store contexts in structs.
- Error types: use `errors.Is()`/`errors.As()` for comparison. Define sentinel errors with `errors.New()`.
- Modules: use Go modules. Commit `go.sum`. Use semantic version tags. Prefer stdlib over third-party when reasonable.
- Concurrency: prefer channels over mutexes. Use `sync.WaitGroup` for fan-out. Guard shared state.
- Benchmarking: use `testing.B` benchmarks. Profile with `go tool pprof`. Use `benchstat` for comparison.
