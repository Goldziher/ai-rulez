---
priority: high
---
Prefer `task` commands over raw build/test/lint commands when a Taskfile.yaml exists. Task runners provide consistent, documented workflows. Use `task --list` to discover available tasks. Always check for a Taskfile before running manual commands. Standard task names: setup, build, test, lint, format, bench — prefer these conventions. Lock files always committed for reproducible builds.
