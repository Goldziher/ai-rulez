---
description: "Closes an accounting period in dry-run first, then for real after confirmation."
argument-hint: "[period]"
---

# Close period

1. Run `billing close --period $ARGUMENTS --dry-run --idempotency-key <key>` and show the result to the user.
2. After the user confirms, run the same command without `--dry-run`.

## Done when

The period shows status `closed` in `billing status`.
