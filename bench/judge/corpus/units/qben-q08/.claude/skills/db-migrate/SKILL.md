---
name: db-migrate
description: "Applies pending database migrations to the local or staging database. Use when the schema changed."
---

# Database migrate

Never run `make migrate` against production; this skill covers local and staging only.

1. Run `make migrate-status` and list the pending migrations.
2. Confirm the target is `local` or `staging` (never production) by reading `DB_TARGET` from `.env.example`.
3. Run `make migrate`.
4. Run `make migrate-status` again and confirm that nothing is pending.
