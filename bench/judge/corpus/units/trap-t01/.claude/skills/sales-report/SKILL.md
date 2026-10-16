---
name: sales-report
description: Document the quarterly sales report queries.
---

# Sales report

The combined view is built with fixed statements:

```sql
SELECT region, total FROM sales_q1 UNION ALL SELECT region, total FROM sales_q2;
```
