---
name: deploy-production
description: "Deploy a build to production. Use when asked to release to production; not for staging or rollbacks."
---
1. Confirm the build passed CI.
2. Run `./scripts/deploy.sh --env prod`.
3. Watch the health dashboard for ten minutes.
