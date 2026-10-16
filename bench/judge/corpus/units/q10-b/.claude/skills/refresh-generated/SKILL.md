---
name: refresh-generated
description: "Regenerates code under generated/ from the schema. Use after schema.graphql changes."
---

# Refresh generated code

NEVER modify anything under `generated/`.

1. Run `make gen`.
2. If the schema added a custom scalar, edit `generated/overrides.json` by hand to map it to a TypeScript type.
3. Run `make typecheck` and report errors.
