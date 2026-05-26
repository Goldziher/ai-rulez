---
priority: high
---

Prefer subagents for non-trivial work — implementation, research, file exploration. Parallelize aggressively — launch independent subagents in a single message. Always critically review subagent output — check actual file changes, verify correctness, fix issues before reporting done. Never trust subagent summaries at face value; the summary describes intent, not necessarily what happened. Work in iterations: delegate → critically review → fix → verify. Run tests after every change — never assume code works without verification.
