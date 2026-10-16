---
name: regen-api-client
description: "Regenerates the TypeScript API client with `make client`. Use after api/openapi.yaml changes; not for server stubs."
---

# Regenerate the API client

1. Run `make client`.
2. Run `git diff --stat` and report which generated files changed.
3. If `make client` exits non-zero, show the last 20 lines of its output and stop.
