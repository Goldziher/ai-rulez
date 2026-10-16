---
paths: ["ledger/**/*.py", "migrations/*.sql"]
---

# Ledger code

- Ledger functions MUST take and return integer cents.
- Every ledger entry MUST record the `tenant_id` of the account.
