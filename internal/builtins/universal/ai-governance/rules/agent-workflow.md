---
priority: high
---

Delegate non-trivial implementation, research and file exploration to subagents, launching independent ones in a single message. Never trust a subagent summary — it describes intent, not what happened; read the actual file changes and fix what is wrong before reporting done. Iterate: delegate → review → fix → verify. Give each subagent a self-contained brief (goal, files, constraints, what to verify and what to return). Run risky or long-lived parallel work in its own git worktree, and check you are in the right one before edits. When a finished subagent has a large context, start a fresh one with a short brief of what changed and what to check instead of resuming it, because resuming re-writes its whole history to the cache.
