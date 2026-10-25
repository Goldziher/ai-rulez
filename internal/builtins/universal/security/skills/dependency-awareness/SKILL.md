---
name: dependency-awareness
description: "Dependency vulnerability audits per language: cargo audit/deny, pip-audit, pnpm audit, govulncheck, bundler-audit, composer audit, dotnet, mix_audit. Use when adding or updating dependencies or triaging CVEs."
---

Audit dependencies before adding them. Prefer well-maintained, widely-used packages with active maintenance. Pin versions and commit lock files. Use language-specific audit tools in CI:

- Rust: `cargo audit`, `cargo deny` (license + advisory policies)
- Python: `pip-audit`, `bandit` (SAST)
- JavaScript/TypeScript: `npm audit`, `pnpm audit`
- Go: `govulncheck`
- Ruby: `bundler-audit`
- PHP: `composer audit`
- Java: OWASP `dependency-check` Maven/Gradle plugin
- C#: `dotnet list package --vulnerable`
- Elixir: `mix_audit`
  Zero tolerance for critical/high CVEs. Automate dependency update PRs where possible.
