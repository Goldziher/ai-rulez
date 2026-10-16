---
paths: ["pkg/**/*.go"]
---

# Go package conventions

- Exported functions have a doc comment that starts with the function name.
- Wrap returned errors with `fmt.Errorf("load config: %w", err)`; do not log and return the same error.
