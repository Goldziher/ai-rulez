<!-- excerpt of https://docs.augmentcode.com/cli/interactive.md (2026-10-18) -->
### Shortcuts

| Command | Description |
| :- | :- |
| `Ctrl + P` | Enhance your prompt with codebase context |
| `Escape` | Interrupt the active agent |
| `Ctrl + C` | Interrupt the active agent |
| `Escape` + `Escape` | Clear the input box |
| `Ctrl + C` | Press twice to exit |
| `Ctrl + D` | Press twice to exit |
| `Up Arrow` | Cycle through previous messages |
| `Down Arrow` | Cycle through previous messages |
| `Ctrl + O` | Open current input in external editor, inserts text on exit |

### Slash Commands

The command menu in interactive mode reflects your current feature flags, available integrations, and loaded plugins. Some commands only appear when the relevant feature is enabled.

#### Common slash commands

| Command | Description |
| :- | :- |
| `/about` | Show system and environment information |
| `/account` | Show account information |
| `/ask` | Enter Ask Mode to limit the agent to retrieval and non-editing tools |
| `/btw <question>` | Ask a side question without committing it to the main conversation (the question argument is required) |
| `/clear` | Clear the display while keeping the current conversation history |
| `/config` | Configure shell, startup script, chat input completions, auto-update, and notifications |
| `/copy` | Copy the request ID or the latest response to the clipboard |
| `/editor` | Open current input in external editor, inserts text on exit |
| `/exit` | Exit Auggie |
| `/feedback` | Submit feedback about the last response |
| `/fork` | Fork the current session into a new conversation, preserving history up to this point |
| `/github-workflow` | Generate a GitHub Action workflow |
| `/help` | Show help |
| `/image [path]` | Attach an image to the next message |
| `/logout` | Logout of Augment |
| `/mcp` | View the status of all configured MCP servers |
| `/new` | Start a new conversation with no message history |
| `/permissions` | View and manage tool permissions |
| `/rename <name>` | Rename the current session |
| `/request-id` | Show the request ID for the current conversation |
| `/rules` | View loaded rules and their attachment status |
| `/sessions` | Resume a previous session |
| `/share` | Generate and copy a shareable link for this session |
| `/stats` | Show current session statistics, including messages, tools, and credits |
| `/status` | Show system status, including MCP servers and rules |
| `/task` | Open task manager to add, edit, and manage tasks |
| `/verbose` | Toggle verbose output for tools |
| `/vim` | Toggle Vim mode for advanced text editing |

#### Additional slash commands you may see

| Command | Description |
| :- | :- |
| `/add-workspace [path]` | Add another workspace folder for multi-folder support |
| `/agents` | Browse available sub-agents |
| `/bash` | Manage running shell processes |
| `/canvas` | Select the context canvas for the session |
| `/context` | Show context usage and token breakdown |
| `/diffs` | View file changes made in the current session |
| `/hooks` | View configured hooks and supported hook event types |
| `/model` | Select the model for the current session |
| `/persona` | Select the persona for the current session |
| `/plan [open]` | Toggle Plan Mode or open the latest plan file |
| `/plugins` | Browse and manage plugins and marketplaces |
| `/queue` | Enter Queue Mode to manage queued messages |
| `/pause` | Pause queue execution |
| `/resume` | Resume paused queue execution |
| `/restart-mcp` | Restart all MCP servers |
| `/skills` | Browse loaded skills, their sources, and token counts |

#### Custom and skill slash commands

In addition to the built-in commands above, the slash menu also lists:

* Your [custom slash commands](/cli/custom-commands) — for example, `.augment/commands/deploy-staging.md` becomes `/deploy-staging`.
* Your discovered [skills](/cli/skills) — for example, a skill at `.augment/skills/my-skill/SKILL.md` becomes `/my-skill`. The `/skills` command opens a browser popover to discover what is available, but it is not required in order to invoke a skill.

Built-in slash commands take precedence over custom commands or skills with the same name, so choose names that do not collide with the commands listed above.
