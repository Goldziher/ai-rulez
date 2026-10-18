<!-- Excerpt of https://docs.poolside.ai/cli/cli-reference.md, fetched 2026-10-18 -->

## pool

| Flag | Short flag | Description |
| - | - | - |
| `--directory` | `-C` | Working directory |

## Slash commands

Slash command availability depends on where you use `pool`.

### Interactive slash commands

Use these commands in an interactive `pool` session.

| Command | What it does |
| - | - |
| `/model` | Open the model selector for the current session |
| `/agent-mode [build\|plan]` | Open the agent mode selector, or switch between Build and Plan. Use `/collaboration-mode` as an alias. Available only when the connected agent server provides agent modes. |
| `/thought-level [<level>]` | Open the thought-level selector, or set a thought level. Use `/effort` as an alias. Available only when the connected agent server provides thought-level choices. |
| `/new` or `/clear` | Clear conversation history and start a new session. The new session keeps the current model and approval mode. Select the agent mode again if needed. |
| `/quit` or `/exit` | Exit the session |
| `/debug:dump` | Write the raw agent-server message log to a local JSON file |

When you use the Poolside agent server, these additional commands are available in interactive mode:

| Command | What it does |
| - | - |
| `/plan` | Switch to plan mode |
| `/skills` | Refresh and list available skills, including any skill load errors |

In interactive `pool`, skills use the `$` skill picker instead of the slash-command menu. Type `$` at the start of the prompt or after a space to open the skill picker and add a skill reference to your prompt.

### ACP slash commands

When your editor passes slash commands to `pool acp`, these commands are available:

| Command | What it does |
| - | - |
| `/plan` | Switch to plan mode when plan mode is available |
| `/clear` | Clear conversation history and free up context |
| `/skills` | Refresh and list available skills, including any skill load errors |

Compatible ACP clients can expose available skills from `pool acp` as slash commands. The available skill commands depend on the skills configured for the agent and workspace.

