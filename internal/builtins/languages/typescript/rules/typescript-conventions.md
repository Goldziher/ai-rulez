---
priority: high
---

- `strict: true` + `noUncheckedIndexedAccess` in tsconfig, never `any` — use `unknown` with type guards.
- ESM imports only, `const` over `let`, `as const` for literals, `interface` over `type` for objects.
- `import type` for type-only imports to avoid runtime overhead. Discriminated unions for type-safe state.
- Formatting/linting: a formatter + linter (e.g., Prettier + ESLint, or Biome). Type checking: `tsc --noEmit` in CI.
- Testing: a fast test runner (e.g., Vitest or Jest), 80%+ coverage. Runtime validation at system boundaries with a schema validator (e.g., Zod).
- Error handling: discriminated unions for expected errors, throw only for unexpected.
- Package manager: a lockfile-based package manager (npm, pnpm, or yarn) with the lockfile committed, build: a bundler (e.g., tsup or esbuild).
- Monorepo: workspace protocol (`workspace:*`), shared tsconfig base, a workspace manifest (e.g., `pnpm-workspace.yaml`).
- Node.js: `node:` prefix for core modules, `fetch` over `axios`.
- Security: `npm audit`/`pnpm audit` for dependency CVE scanning. Zero tolerance for critical/high vulnerabilities.
- Anti-patterns: non-null assertions (`!`), type assertions (`as`), `enum` (use unions), `@ts-ignore`.
