# Billing service

- Amounts are integer cents; NEVER use floating point for money.
- Every refund or charge call MUST pass an `idempotency_key`.
- A command that changes balances MUST be run with `--dry-run` first and the result shown to the user.
