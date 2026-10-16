---
paths: ["**/*.go"]
---

# Go errors

- Wrap errors with `fmt.Errorf("<action>: %w", err)`.
- Compare errors with `errors.Is` or `errors.As`, not `==` or string matching.
