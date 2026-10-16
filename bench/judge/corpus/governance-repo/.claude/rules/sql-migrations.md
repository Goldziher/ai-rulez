---
paths: ["**/migrations/*.sql"]
---

# SQL migrations

- NEVER edit a migration that has been merged; add a new numbered file.
- Migration files MUST be named `NNNN_description.sql`.
- Every query on tenant data MUST carry a `tenant_id` predicate.
