---
name: sql-explain
description: "Explains a PostgreSQL EXPLAIN ANALYZE plan and points at the slowest node. Use when the user pastes a query plan; not for rewriting schemas."
---

# SQL explain

1. Identify the node with the highest actual time and its row estimate versus actual rows.
2. If the estimate is off by more than 10x, say that statistics are likely stale and suggest `ANALYZE <table>`.
3. If a sequential scan filters most rows away, suggest one index and show the `CREATE INDEX` statement without running it.
4. Finish with a three-line summary: slowest node, cause, suggested change.
