---
targets: ["copilot", "claudecode-plugin"]
description: "Workflow security"
globs: [".github/workflows/*.yml"]
copilot:
  name: "Actions security"
  excludeAgent: "code-review"
---

Pin every action to a commit SHA.
