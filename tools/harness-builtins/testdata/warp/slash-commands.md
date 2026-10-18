<!-- Excerpt of https://docs.warp.dev/agents/capabilities/slash-commands.md, fetched 2026-10-18 -->

## Static slash commands

Warp currently supports the following built-in Slash Commands:

| Slash Command | Description |
| --- | --- |
| `/add-mcp` | Add a new [MCP server](https://docs.warp.dev/agents/capabilities/mcp/). |
| `/agent` | Start a new [agent conversation](https://docs.warp.dev/agents/local-agents/interacting-with-agents/). Optionally include a prompt to send immediately. |
| `/fork` | [Forks the current conversation](https://docs.warp.dev/agents/local-agents/interacting-with-agents/conversation-forking/) into a new thread with the full context and history of the original. |

You can optionally include a prompt that will be sent immediately in the forked conversation. |
| `/fork-and-compact` | [Forks the current conversation](https://docs.warp.dev/agents/local-agents/interacting-with-agents/conversation-forking/) and automatically compacts the forked version.  

Useful when you want a fresh, summarized starting point that preserves relevant context while trimming the rest. |
| `/new` | Start a new [agent conversation](https://docs.warp.dev/agents/local-agents/interacting-with-agents/) (alias for `/agent`). |
| `/plan` | Prompt the Agent to research and create a [plan](https://docs.warp.dev/agents/capabilities/planning/) for a task. When the work benefits from parallel agents, the plan can include [multi-agent orchestration](https://docs.warp.dev/platform/orchestration/). |
| `/skills` | Invoke a [skill](https://docs.warp.dev/agents/capabilities/skills/) from a searchable menu. |
