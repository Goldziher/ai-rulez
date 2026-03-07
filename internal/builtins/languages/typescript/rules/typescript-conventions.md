---
priority: high
---
# TypeScript Conventions
- Use strict TypeScript configuration (`strict: true`). Avoid `any` — use `unknown` with type guards.
- Prefer `const` over `let`. Never use `var`. Use template literals over string concatenation.
- Use ESM imports. Prefer named exports over default exports.
- Use `vitest` or `jest` for testing. Enable strict null checks.
- Format with Prettier, lint with ESLint.
