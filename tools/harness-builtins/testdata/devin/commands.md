<!-- excerpt of https://docs.devin.ai/cli/reference/commands.md (2026-10-18) -->
## Slash Commands

These commands are available inside an interactive session. Type them at the prompt.

### Mode & Model

| Command | Description |
| - | - |
| `/mode [normal\|accept-edits\|smart\|plan\|bypass]` | Show or switch the current mode (`autonomous` is available in sandbox sessions) |
| `/normal` | Switch to Normal mode (default) |
| `/accept-edits` | Switch to Accept Edits mode (auto-approve file edits in workspace) |
| `/smart` | Switch to Smart mode (auto-approve actions a fast model judges safe). Rolling out gradually — it may not be available on your account yet. |
| `/plan` | Switch to Plan mode (read-only planning) |
| `/ask <question>` | Ask a question without making code changes (oneshot) |
| `/bypass` | Switch to Bypass mode (auto-approve all actions) |
| `/autonomous` | Switch to Autonomous mode (sandbox-enforced; requires `--sandbox`) |
| `/model [name]` | Show or change the current model |
| `/fast` | Switch to SWE-1.6 Fast |
| `/theme [dark\|light\|terminal-dark\|terminal-light\|no-color]` | Switch between themes (dark, light, terminal dark, terminal light, no color) |

<Note>
  `/bypass` has aliases `/yolo` and `/dangerous`. All three do the same thing.
</Note>

### Session Management

| Command | Description |
| - | - |
| `/clear` | Clear conversation history and start a new session. Alias: `/new` |
| `/continue [session-id]` | Resume a previous session |
| `/fork [step]` | Fork the current session to a new session. Optionally fork from a specific step (see `/steps`). |
| `/steps` | List conversation steps (use with `/fork` and `/revert`) |
| `/revert <step>` | Revert file changes from a specific step onwards and rewind the conversation to before that step |
| `/resume [session-id]` | Open the interactive session picker, or resume a specific session by ID |
| `/ls [--all]` | List recent sessions (current directory only by default). Alias: `/list-sessions` |
| `/title <new title>` | Rename the current session |
| `/rename-session <new title>` | Rename the current session |
| `/rm-session <session-id>` | Irreversibly delete a session and all its data |
| `/export` | Show export info. Use the `--export` CLI flag to enable conversation export. |
| `/exit` | Exit the application (alias: `/quit`). You can also type `exit` or `quit` without the `/` prefix. |

### Workspace

| Command | Description |
| - | - |
| `/workspace` | List workspace directories (alias: `/workspaces`) |
| `/add-dir <path>` | Add an additional workspace directory |
| `/undo-add-dir <path>` | Remove a workspace directory |
| `/remove-dir <path>` | Remove a workspace directory (second name for `/undo-add-dir`) |

### Automation

| Command | Description |
| - | - |
| `/loop <prompt>` | Run a prompt then auto-review the diff in a loop |
| `/btw <prompt>` | Ask a quick side question. Runs a sidechain using the current conversation context and prints the answer in a box, without adding the question to the main conversation. |

### Extensibility

| Command | Description |
| - | - |
| `/hooks` | List all loaded hooks with their IDs, event types, and source paths |
| `/mcp` | List configured MCP servers and their status |

### Utilities

| Command | Description |
| - | - |
| `/help` | Show available slash commands |
| `/shortcuts` | Browse keyboard shortcuts in an interactive, searchable list grouped by category. Press `Enter` on a row to [rebind it](/cli/reference/keyboard-shortcuts#customizing-keybindings), and see each action's `context.action` identifier |
| `/config` | Open the interactive config editor |
| `/bug [description]` | Report a bug to the Devin CLI developers |
| `/update [--force]` | Check for and install updates. Pass `--force` to re-install even when already on the latest version. |
| `/upgrade` | Upgrade your subscription plan |
| `/login` | Authenticate with your account |
| `/logout` | Clear stored credentials and exit |
| `/login-status` | Show login and authentication status (advertised to ACP hosts as `/status`) |
| `/org` | Select your Devin organization |
| `/copy` | Copy the last response to the clipboard |
| `/feedback <up\|down>` | Rate the last response |
| `/mouse` | Toggle mouse event capture (click/hover/scroll in menus) |
| `/context` | Show context window usage |
| `/usage` | Show estimated credit/ACU usage for the session, including usage from previous openings of a resumed session |
| `/session-stats` | Show session statistics (alias: `/stats`) |
| `/compact` | Force conversation compaction |

#### Session statistics

`/session-stats` (alias `/stats`) is the full view of what a session has consumed, where `/usage` is the thin credit/ACU summary. It renders every usage dimension the server reports — credits, ACUs, agent messages, turn continuations, and token usage — using the server's own labels and grouping, so new dimensions show up without a CLI update. The `Model` row names the model that actually served the billed turns, which can differ from the model you selected. Totals persist across resume, so a resumed session reports its cumulative usage rather than starting over.

### Cloud Sessions

See [Cloud CLI](/cli/cloud) for a guide to running Devin Cloud sessions from the terminal.

| Command | Description |
| - | - |
| `/cloud` | Switch a fresh local session to run in Devin Cloud (before the first message; run `/clear` first otherwise) |
| `/handoff [task]` | Local session: continue the task in a cloud session. Cloud session: check out the session's pull request branch and continue locally. See [Hand off to cloud Devins](/cli/handoff). |
| `/repo` | Pick the repositories for this cloud session |
| `/platform` | Pick the platform (OS) for this cloud session |
| `/open [web\|desktop]` | Open this cloud session in the web app (default) or Devin Desktop |
| `/ssh` | SSH into this cloud session's VM. See [SSH](/cli/ssh). |
| `/pickup` | Check out this cloud session's pull request branch and continue locally (same as `/handoff` in a cloud session) |
| `/rename <new title>` | Rename this cloud session |
| `/archive` | Archive this cloud session |

***
