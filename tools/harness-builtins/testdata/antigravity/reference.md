<!-- excerpt of https://antigravity.google/docs/cli/reference.md (2026-10-18); rows dropped -->
# CLI commands

## Core slash commands

Type `/` inside the prompt box to open the typeahead command selection menu.

| Command | Category | Alias | Execution Purpose |
| :-- | :-- | :-- | :-- |
| **`/add-dir <path>`** | Utilities | — | Add a directory path to the active workspace. |
| **[`/agents`](/docs/cli/commands/agents)** | Tools & Tasks | — | Open the [Agent Manager panel](/docs/cli/commands/agents) to switch custom agents and monitor background subagents. |
| **[`/btw`](/docs/cli/commands/btw) `<query>`** | Utilities | — | Ask a [side question in the background](/docs/cli/commands/btw) without interrupting the main conversation. |
| **`/clear`** | Utilities | `/new` | Clear the terminal and reset active conversation contexts. |
| **`/copy [n|btw]`** | Utilities | — | Copy the last agent response, the n-th most recent response, or the latest [`/btw`](/docs/cli/commands/btw) answer. |
| **`/exit`** | Core | `/quit` | Close the TUI session and restore your host shell. |
| **`/fork`** | Conversations | `/branch` | Clone the current conversation thread into a new parallel session. |
| **[`/resume`](/docs/cli/commands/resume)** | Conversations | `/switch`, `/conversation` | Open the [conversation picker overlay](/docs/cli/commands/resume) to select and load previous threads. |
| **[`/teamwork-preview`](/docs/teamwork)** `<task>` | Reasoning | `/teamwork` | Launch [collaborative multi-agent teams](/docs/teamwork) for long-horizon projects (paid plans). |
| **[`/voice`](/docs/cli/commands/voice)** | Utilities | `/record` | Dictate a prompt using your microphone. |

## Default keybindings

| Key | TUI Command | Action Behavior |
| :-- | :-- | :-- |
| `Ctrl+C` | `/exit` | Exit |
