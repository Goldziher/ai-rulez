---
name: secrets-policy
description: "States the secrets policy for contributors. Use when asked how credentials are handled."
---

# Secrets policy

NEVER commit credentials, tokens or private keys; read them from `ACME_*` environment variables.

Secrets MUST NOT have a default value; fail fast when one is missing.
