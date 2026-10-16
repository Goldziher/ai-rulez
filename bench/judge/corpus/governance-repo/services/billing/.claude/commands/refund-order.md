---
description: "Refunds one order in dry-run first, then for real after confirmation."
argument-hint: "<order-id>"
---

# Refund order

1. Run `billing refund --order $ARGUMENTS --dry-run --idempotency-key <key>` and show the result to the user.
2. After the user confirms, run the same command without `--dry-run`.

## Done when

The refund id is printed and the order shows status `refunded`.
