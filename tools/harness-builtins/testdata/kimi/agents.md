---
url: /kimi-code/en/customization/agents.md
---

<!-- Excerpt of https://moonshotai.github.io/kimi-code/en/customization/agents.md (fetched 2026-10-18) -->
# Agents and Sub-Agents

Every session in Kimi Code CLI is driven by a **main Agent**. The main Agent understands the user's intent, plans steps, calls tools, and when needed dispatches **sub-agents** to handle more focused sub-tasks, such as exploring an unfamiliar codebase, reviewing multiple implementations in parallel, or planning a large refactor without touching the main context.

A sub-agent receives a task description from the main Agent, works in its own isolated context, and then returns its conclusions. It does not communicate with the user directly, and its intermediate reasoning and tool call records do not mix into the main Agent's history.

## Built-in Sub-Agents

Kimi Code CLI includes three built-in sub-agents, ready to use out of the box, each aimed at a different task shape:

* **`coder`**: The default sub-agent, a general-purpose software engineering assistant that can read and write files, execute commands, search code, and land concrete changes.
* **`explore`**: Dedicated to codebase exploration; performs read-only operations only and does not modify any files. Ideal for quickly searching, reading, and summarizing a repository without touching files.
* **`plan`**: Dedicated to implementation planning and architecture design; even shell commands are not available, keeping the focus on "figuring out how to do something" rather than "actually doing it."

Beyond the three types, three conventions govern how sub-agents work: tool boundaries, delegation depth, and completion timing.

A `coder` sub-agent shares most of the main Agent's tool set: it can run shell commands in the background, maintain todo lists, enter Plan mode, and invoke Agent Skills. The three built-in sub-agents cannot dispatch further sub-agents.

By default a custom agent inherits the built-in delegation allowlist (`coder`, `explore`, `plan`), whose members cannot dispatch further either, so delegation chains always terminate and unbounded recursive spawning is impossible without an explicit opt-in. A custom agent can opt into deeper chains by declaring an explicit [`subagents`](#agent-file-format) allowlist.
