---
name: vite-plus-conventions
description: vite+ unified TypeScript toolchain conventions: the vp CLI for package/node management, oxlint/oxfmt, type-aware linting, vitest, rolldown/tsdown bundling, task caching, and migration. Load when configuring or reviewing a vite+ TypeScript toolchain.
---

- Use [vite+](https://viteplus.dev) as the unified TypeScript toolchain. It wraps best-in-class tools behind a single CLI. Configure your entire toolchain in `vite.config.ts` by importing `defineConfig` from `vite-plus`.
- Package manager: vite+ auto-detects from `package.json` engine field or lockfile (`pnpm-lock.yaml`, `yarn.lock`, `package-lock.json`, `bun.lock`). Fallback is `pnpm`. Run `vp install`, `vp add <pkg-name>` or `vp remove <pkg-name>` for managing package dependencies.
- Node manager: vite+ manages Node.js versions — replaces `nvm`/`fnm`. Run `vp env on` then `vp env install <node-version>` to install Node.js runtime. Run `vp env pin` to pin version.
- Linter: `oxlint` via vite+ — a fast Rust-powered alternative to legacy JavaScript linters. Run `vp lint` for linting with TypeScript-aware rules.
- Formatter: `oxfmt` via vite+ — a fast alternative to legacy JavaScript formatters. Run `vp fmt` for consistent code formatting.
- Type checker: `oxlint` with type-aware linting via vite+ — replaces `tsc --noEmit` for lint-time type checks. Configure it by enabling both `lint.options.typeAware` and `lint.options.typeCheck` in `vite.config.ts`. Run `vp lint` for type-aware linting.
- Testing: `vitest` via vite+ — run `vp test` for fast Vite-native testing with HMR, coverage, and snapshot support. Import from `vite-plus/test` instead of `vitest` when writing tests.
- Bundler: `rolldown` (Rust-powered Rollup replacement) for Vite apps, `tsdown` for library builds. Run `vp build` for Vite apps or `vp pack` for libraries.
- Task runner: vite+ supports task caching for build pipelines. Define tasks in `vite.config.ts` or `package.json` scripts and benefit from incremental builds by using `vp run <task-name>`.
- CI integration: use `vp lint`, `vp fmt`, `vp test --coverage`, `vp build`/`vp pack` in CI pipelines.
- Migrate from existing tools: vite+ is a drop-in replacement. Run `vp migrate` to migrate your entire toolchain to vite+.
