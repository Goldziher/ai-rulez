---
name: deploy-staging
description: "Deploy a build to the staging environment. Use when asked to ship to staging; not for production or rollbacks."
---
1. Confirm the build passed CI.
2. Run `./scripts/deploy.sh --env staging`.
3. Run the smoke tests against staging.
