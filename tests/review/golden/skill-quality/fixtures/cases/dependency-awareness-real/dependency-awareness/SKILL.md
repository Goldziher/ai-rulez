---
name: dependency-awareness
description: Per-language dependency vulnerability audit tool reference (cargo audit/deny, pip-audit, npm/pnpm audit, govulncheck, bundler-audit, composer audit, OWASP dependency-check, dotnet vulnerable, mix_audit). Load when adding, updating, or auditing project dependencies, wiring up CI supply-chain checks, or triaging CVEs in a lock file.
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
