---
name: db-backup
description: "Creates a snapshot of the development database. Use before risky migrations; not for restores, use `db-restore`."
---

# Database backup

1. Run `make db-snapshot`.
2. Report the snapshot file name printed on the last line.
