<!-- excerpt of https://docs.devin.ai/cli/subagents.md (2026-10-18) -->
## Subagent Profiles

Each subagent runs with a specific profile that determines its capabilities. There are two built-in profiles:

| Profile | Description | Tool Access | Model |
| - | - | - | - |
| `subagent_explore` | Read-only codebase exploration and research | Read-only codebase tools plus web search; cannot edit files or fetch arbitrary URLs (regardless of foreground or background) | Default subagent model (router-selected unless an admin pins one) |
| `subagent_general` | General-purpose tasks including code changes | Full tool access (foreground) or pre-approved tools only (background) | Same model as the parent agent |

