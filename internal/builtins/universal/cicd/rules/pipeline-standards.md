---
priority: medium
---

CI stages: Validate → Build → Test → Deploy. Quality gates: zero warnings, tests pass, coverage thresholds. OS matrix: test on Linux, macOS, Windows when applicable. Workflows use task runner commands (task, make) — never raw build commands. Tag-based releases trigger multi-platform builds. Lock files always committed — reproducible builds.
