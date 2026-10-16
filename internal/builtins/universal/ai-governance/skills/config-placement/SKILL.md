---
name: config-placement
description: "Where to put a new instruction in AI-assistant configuration (always-loaded root file, path-scoped rule, directory-level context file, skill, agent) and how to review a rule or skill. Load when adding or reviewing a rule, skill, agent or context file."
---

## Placement order

- Always needed and short: the root context file.
- Tied to files, stable and under about 50 lines: a path-scoped rule in the deepest directory that covers all its globs.
- Knowledge about one directory: that directory's context file.
- A procedure or reference: a skill next to its code.
- Isolation or restricted tools: an agent.

## Rules of thumb

- One home per fact. Link to it, do not copy it.
- A rule covers one topic and is imperative and checkable.
- Check that every glob matches a tracked file and that every path, command and flag the text names exists.
- Reviewers that run in CI often do not read path-scoped rules, so repeat a review-critical convention as a short block in the context file of its area.
- Never put secrets or customer data in any of these files.
