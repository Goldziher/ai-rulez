---
name: db-restore
description: "Restores the development database from a snapshot. Use when a migration went wrong; not for creating snapshots, use `db-backup`."
---

# Database restore

1. Ask the user which snapshot to use and wait for the answer.
2. Run `make db-restore SNAPSHOT=<name>`.
3. Run `make migrate-status` and report the result.
