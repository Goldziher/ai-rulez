# Secrets and data

- NEVER commit credentials, tokens or private keys; read them from `ACME_*` environment variables.
- Secrets MUST NOT have a default value; fail fast when one is missing.
- Customer data MUST NOT be pasted into prompts, skills or fixtures.
