<!-- Excerpt of https://docs.qoder.com/cli/builtins-reference.md, fetched 2026-10-18 -->

## Built-in Subagents

Subagents are independent execution units spawned by the main agent to handle specific types of tasks in isolation.

| Agent              | Purpose                                                                                                                      | Constraints                                                                                  |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `general-purpose`  | General-purpose agent for handling most user requests and complex multi-step tasks.                                          | Access to all tools.                                                                         |
| `Explore`          | Explore agent for read-only code retrieval and understanding; thoroughness can be specified (fast / medium / very thorough). | Read-only; write and state-changing tools are disabled; uses a lightweight model.            |
| `qoder-guide`      | Answers questions about Qoder CLI features and usage.                                                                        | Available only in the interactive interface (not registered in SDK mode); read-only toolset. |

> You can use the `agents.overrides` configuration item to override models, tools, or runtime configurations for specific agents, or to disable an agent. For custom agents, see [Subagents](/cli/builtins-reference).

## Built-in Bundled Skills

Skills are reusable templates for specialized capabilities, invoked via `/skill-name` or automatically called by the agent on demand.

| Skill                 | Purpose                                                                                                                                                                                                                                 |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `loop`                | Executes prompts or slash commands repeatedly at fixed intervals. See [Loop Command Reference](/cli/loop-reference).                                                                                                                    |
| `run`                 | Launches and drives the project's actual application to observe the effects of changes at runtime.                                                                                                                                      |
| `workflow-authoring`  | Reference for writing Workflow scripts (requires workflows and Skills availability); loading the reference does not authorize workflow execution.                                                                                       |

## Next Steps

- Overview of built-in capabilities: [Built-in Capabilities Overview](/cli/built-ins).
- Custom skills: [Skills](/cli/builtins-reference).
- Custom agents: [Subagents](/cli/builtins-reference).
