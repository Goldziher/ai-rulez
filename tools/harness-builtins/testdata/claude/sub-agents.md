<!-- Excerpt of https://code.claude.com/docs/en/sub-agents.md (the "Built-in subagents" section, fetched 2026-10-27) -->

## Built-in subagents

Claude Code includes built-in subagents that Claude automatically uses when appropriate. Each inherits the parent conversation's permission rules; most run with a restricted tool set.

Explore and Plan start without your CLAUDE.md files and the git status snapshot to keep research fast and inexpensive. Other built-in and [custom subagents](#configure-subagents) load both, unless you set [`omitClaudeMd`](#supported-frontmatter-fields) in a custom subagent's definition to start it without the user, project, and local CLAUDE.md files. For the full breakdown of what reaches a subagent, see [what loads at startup](#what-loads-at-startup).

**Explore**

A fast, read-only agent optimized for searching and analyzing codebases.

* **Model**: the main conversation's model. When the main conversation runs Fable, Explore's model depends on how you connect:
  * With a Claude subscription, an Anthropic Console account, or an [LLM gateway](https://code.claude.com/docs/en/llm-gateway) reached through `ANTHROPIC_BASE_URL`, Explore runs on the Opus model that the [`opus` alias](https://code.claude.com/docs/en/model-config#model-aliases) resolves to.
  * On Amazon Bedrock, Google Cloud's Agent Platform, Microsoft Foundry, [Claude Platform on AWS](https://code.claude.com/docs/en/claude-platform-on-aws.md), or a [Claude apps gateway](https://code.claude.com/docs/en/claude-apps-gateway), Explore stays on the main conversation's model.
* **Tools**: read-only tools; Write and Edit are denied
* **Purpose**: file discovery, code search, codebase exploration

A [user or project subagent](#choose-the-subagent-scope) named `Explore` overrides the built-in and keeps its own `model` field, so define one with `model: haiku` to run exploration on a lower-cost model. To force one model onto every subagent, Explore included, see [Run every subagent on one model](#run-every-subagent-on-one-model).

Claude delegates to Explore when it needs to search or understand a codebase without making changes. This keeps exploration results out of your main conversation context.

When invoking Explore, Claude specifies a thoroughness level: **quick** for targeted lookups, **medium** for balanced exploration, or **very thorough** for comprehensive analysis.

**Plan**

A research agent used during [plan mode](https://code.claude.com/docs/en/permission-modes.md#analyze-before-you-edit-with-plan-mode) to gather context before presenting a plan.

* **Model**: inherits from the main conversation, unless you set `CLAUDE_CODE_SUBAGENT_MODEL` and [force it onto every subagent](#run-every-subagent-on-one-model)
* **Tools**: read-only tools; Write and Edit are denied
* **Purpose**: codebase research for planning

When you're in plan mode and Claude needs to understand your codebase, it delegates research to the Plan subagent so that exploration output stays in a separate context window while the main conversation remains read-only.

**General-purpose**

A capable agent for complex, multi-step tasks that require both exploration and action.

* **Model**: the [`CLAUDE_CODE_SUBAGENT_MODEL`](#choose-a-model) model if you set one and nothing assigns a model another way, otherwise the main conversation's model; [Choose a model](#choose-a-model) states the full order, and [Run every subagent on one model](#run-every-subagent-on-one-model) shows how to make the variable override those sources
* **Tools**: every tool [available to subagents](#available-tools)
* **Purpose**: complex research, multi-step operations, code modifications

Claude delegates to general-purpose when the task requires both exploration and modification, complex reasoning to interpret results, or multiple dependent steps.

**Other**

Claude Code includes additional helper agents for specific tasks. These are typically invoked automatically, so you don't need to use them directly.

| Agent | Model | When Claude uses it |
| :- | :- | :- |
| claude | None of its own; follows the [model order](#choose-a-model) when Claude spawns it as a subagent | When a task doesn't fit a more specialized agent. A catch-all with every tool [available to subagents](#available-tools). Also the default agent for a dispatched [background session](https://code.claude.com/docs/en/agent-view.md); [which permission mode it starts in](https://code.claude.com/docs/en/agent-view.md#permission-mode-model-and-effort) depends on how the session was started |
| statusline-setup | Sonnet | When you run `/statusline` to configure your status line |
| claude-code-guide | Haiku | When you ask questions about Claude Code features |

Built-in subagents are registered by default in interactive sessions. To restrict them:

* To block a specific built-in type, add it to `permissions.deny` as shown in [Disable specific subagents](#disable-specific-subagents).
* To prevent Claude from delegating to any subagent, deny the `Agent` tool itself with [`permissions.deny`](https://code.claude.com/docs/en/permissions#tool-specific-permission-rules).
* To remove only the built-in `Explore` and `Plan` subagents, set [`CLAUDE_CODE_DISABLE_EXPLORE_PLAN_AGENTS=1`](https://code.claude.com/docs/en/env-vars). Claude reads and explores files directly instead of delegating to them. Requires Claude Code v2.1.198 or later.
* In [non-interactive mode](https://code.claude.com/docs/en/headless.md) and the [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview.md), set [`CLAUDE_AGENT_SDK_DISABLE_BUILTIN_AGENTS=1`](https://code.claude.com/docs/en/env-vars) to remove all built-in types and supply only your own.

An Agent tool call that omits `subagent_type` fails with [`subagent_type is required`](https://code.claude.com/docs/en/errors#subagent-type-is-required) when the session has no `general-purpose` subagent to fall back on.

Beyond these built-in subagents, you can create your own with custom prompts, tool restrictions, permission modes, hooks, and skills. The following sections show how to get started and customize subagents.
