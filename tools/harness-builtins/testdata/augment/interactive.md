<!-- excerpt of https://docs.augmentcode.com/cli/interactive.md (2026-11-05) -->
### Shortcuts

Run `/hotkeys` to see every shortcut. You can customize shortcuts in `~/.augment/keybindings.json` and apply changes with `/reload`.

| Command | Description |
| :- | :- |
| `Ctrl + P` | Enhance your prompt with codebase context |
| `Escape` | Interrupt the active agent |
| `Escape` + `Escape` | Open the session tree (configurable with `doubleEscapeAction`) |
| `Ctrl + C` | Clear the input box |
| `Ctrl + D` | Exit when the input box is empty |
| `Shift + Tab` | Cycle the reasoning effort on reasoning models |
| `Ctrl + L` | Open the model selector |
| `Ctrl + O` | Expand or collapse tool output |
| `Ctrl + T` | Show or hide thinking blocks |
| `Ctrl + G` | Open current input in external editor, inserts text on exit |
| `Alt + Enter` | Queue a follow-up message while the agent is working |
| `Alt + Up` | Restore queued messages to the input box |
| `Ctrl + V` | Paste an image from the clipboard (`Alt + V` on Windows) |
| `Ctrl + Z` | Suspend Auggie to the background |
| `Up Arrow` / `Down Arrow` | Cycle through previous messages |

All menus and selectors support type-to-filter. Use the arrow keys to move and `Escape` to go back.

### Slash Commands

The command menu reflects your available integrations and loaded plugins. Some commands only appear when the relevant feature is enabled. Argument hints appear beside commands as you type.

#### Built-in slash commands

| Command | Description |
| :- | :- |
| `/about` | Show version, environment, account, and session info |
| `/agents` | Browse and create custom agents |
| `/changelog` | Show changelog entries |
| `/cloud [on\|off]` | Save the cloud mode startup preference; restart Auggie to apply |
| `/compact [instructions]` | Summarize the conversation to free up context |
| `/context` | Show context usage |
| `/copy` | Copy the last agent message to the clipboard |
| `/effort [level]` | Choose how much a reasoning model thinks |
| `/enhance` | Enhance the current prompt with codebase context |
| `/exit`, `/quit` | Exit Auggie |
| `/export [path]` | Export the session to HTML (default) or JSONL |
| `/feedback` | Rate the latest response and optionally leave a note |
| `/fork` | Create a new fork from a previous message |
| `/hooks` | View configured hooks |
| `/hotkeys` | Show all keyboard shortcuts |
| `/import <path>` | Import and resume a session from a JSONL file |
| `/import-legacy-sessions [ids]` | Import sessions from Auggie 0.x |
| `/login`, `/logout` | Log in or out |
| `/mcp` | View the status of configured MCP servers |
| `/model` | Select the model (press `Tab` to change reasoning effort) |
| `/name <name>` | Set the session display name |
| `/new` | Start a new session |
| `/permissions` | View and manage tool permission rules |
| `/plugins` | Manage plugins and marketplaces |
| `/reload` | Reload keybindings, extensions, skills, prompts, and themes |
| `/restart-mcp` | Restart MCP servers |
| `/resume` | Resume a different session |
| `/rules` | View loaded rules |
| `/scoped-models` | Choose which models to cycle through |
| `/session` | Show session info and stats |
| `/settings` | Open the settings menu |
| `/tree` | Navigate the session tree and switch branches |
| `/worktree` | Select, switch, create, or delete git worktrees |


#### Custom and skill slash commands
