---
name: task-runner
description: "Use `task` (go-task) instead of raw build/test/lint commands in any repository whose root contains a Taskfile.yaml or Taskfile.yml, with the standard target names setup, build, test, lint, format, bench. Load on seeing a Taskfile.yaml/Taskfile.yml in the repository root, or before running a build, test, lint or format command in an unfamiliar repository."
---

- If the repository root has a `Taskfile.yaml` or `Taskfile.yml`, prefer `task <target>` over the raw toolchain command — the Taskfile carries the flags and env the raw command omits.
- `task --list` enumerates the available targets.
- Standard target names: `setup`, `build`, `test`, `lint`, `format`, `bench`. Follow them when adding targets.
- Commit lock files so builds are reproducible.
