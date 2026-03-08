---
priority: medium
---
# Task Automation Overview
Task runners (Taskfile.yaml, Makefile, Just, etc.) standardize project workflows.
- Check for `Taskfile.yaml` or `Makefile` in project root before running raw commands.
- Use `task --list` or `make help` to discover available targets.
- Common patterns: `task build`, `task test`, `task lint`, `task fmt`.
- Task runners ensure consistent flags, environment variables, and execution order across contributors.
- Prefer task definitions over documenting shell commands — they are self-documenting and version-controlled.
