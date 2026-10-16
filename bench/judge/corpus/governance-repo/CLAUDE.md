# Acme monorepo

Acme platform: Python services under `services/`, React apps under `apps/`, shared tooling under `tools/`.

@docs/engineering.md

## Git

- Branch names MUST follow `{user}.{ACME-123}.{slug}` and MUST NOT contain `/`.
- A commit message MUST be a single subject line under 100 characters that starts with the Jira key, for example `ACME-123 fix rounding`, with no body and no trailer.
- Pull requests SHOULD stay under 400 changed lines; PREFER splitting by service.

## JavaScript

- Use pnpm for all installs and scripts. MUST NOT run `npm install` or `yarn` in this repository.

## Commands

- Run the whole test suite with `make test`; one service with `make test SERVICE=billing`.
