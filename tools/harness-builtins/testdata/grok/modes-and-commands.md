<!-- Excerpt of https://docs.x.ai/build/modes-and-commands.md, fetched 2026-10-18 -->

## Core TUI commands

| Command | What it does |
| ------- | ------------ |
| `/quit` (alias `/exit`) | Quit the application |
| `/help` | Browse commands and keyboard shortcuts |
| `/new` (alias `/clear`) | Start a new session |
| `/rename <title>` (alias `/title`) | Rename the current session |
| `/model <name>` (alias `/m`) | Switch the active model |
| `/settings` (alias `/config`) | Open the settings modal |
| `/login`, `/logout` | Sign in, or sign out of the current account |
| `/skills` | Open the unified extensions modal at the Skills tab |
| `/mcps` | Open the unified extensions modal at the MCP tab |

## Shell-provided commands

| Command | What it does |
| ------- | ------------ |
| `/goal <objective>` | Work toward an objective until it is verified complete — see [Goals](#goals) |
| `/flush` | Flush conversation memory to disk now |
| `/memory` (alias `/mem`) | Browse, view, and manage your memories |
| `/dream` | Run memory consolidation |

### Commands

| Command | What it does |
| ------- | ------------ |
| `/goal <objective>` | Start a goal. Add `--budget <tokens>` to cap the tokens it may spend. |
| `/goal` or `/goal status` | Show the goal, its status, tokens used, and time elapsed. |
| `/goal pause` | Pause the goal. |
| `/goal resume` | Continue a paused goal. |
| `/goal clear` | Remove the goal. |

