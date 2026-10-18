<!-- Excerpt of https://docs.x.ai/build/features/subagents.md, fetched 2026-10-18 -->

#### Features

# Subagents

Subagents are independent child sessions with their own context. They return a summary to the parent when finished. Enabled by default when the setting is unset.

## Built-in types

| Type | Role |
| ---- | ---- |
| `general-purpose` | Default full-capability child |
| `explore` | Read, list, and search only (no shell, no edits) |
| `plan` | Drafts an implementation plan (no shell, no edits) |

Add or override types under `.grok/agents/` or `~/.grok/agents/`. Manage agents and personas with `/config-agents` (alias `/agents`) or `/personas`. Personas are behavioral overlays only (tone, focus, contracts); define them under `[subagents.personas]` or `.grok/personas/*.toml` / `~/.grok/personas/*.toml`.
