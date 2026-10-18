<!-- excerpt of https://docs.snowflake.com/en/user-guide/cortex-code/cli-reference.md (2026-10-18) -->
### Slash commands

Slash commands are special commands you type at the interactive prompt. They start with `/` and
control CoCo's behavior without being sent to the AI model as a prompt. Slash commands work only
in interactive mode.

#### Session management

| Command                      | Description                                             |
| ---------------------------- | ------------------------------------------------------- |
| `/help`                      | Show interactive help                                   |
| `/clear`, `/cls`             | Clear the screen                                        |
| `/new`                       | Start a new session                                     |
| `/rename <title>`            | Rename current session                                  |
| `/quit`, `/exit`             | Exit CoCo CLI                                           |
| `/resume`, `/r`, `/sessions` | List and resume sessions                                |
| `/rewind`                    | Go back *n* steps in conversation or pick interactively |
| `/fork`                      | Fork current session into a new session                 |

#### Model and mode

| Command       | Description                                                |
| ------------- | ---------------------------------------------------------- |
| `/model`      | Show/select AI model                                       |
| `/plan`       | Enable plan mode                                           |
| `/plan-off`   | Disable plan mode                                          |
| `/bypass`     | Enable bypass mode (auto-approve all including tool calls) |
| `/bypass-off` | Disable bypass mode                                        |
| `/status`     | Show current configuration                                 |

#### Snowflake and data

| Command                    | Description                              |
| -------------------------- | ---------------------------------------- |
| `/sql <query>`             | Execute SQL query                        |
| `/sql <query> --limit <n>` | Limit displayed rows                     |
| `/table [<file>]`, `/csv`  | Open table viewer                        |
| `/connections`, `/conn`    | Manage Snowflake connections             |
| `/guardrails`              | Configure Restricted Session Scope       |
| `/guardrails status`       | Show the active Restricted Session Scope |

#### Development tools

| Command                        | Description               |
| ------------------------------ | ------------------------- |
| `/sh`, `! <command>`           | Execute shell command     |
| `/diff`, `/changes`, `/review` | Review git changes        |
| `/worktree`                    | Manage git worktrees      |
| `/fdbt`                        | Fast dbt project analysis |
| `/lineage`                     | dbt lineage visualization |

#### Configuration

| Command           | Description                                          |
| ----------------- | ---------------------------------------------------- |
| `/settings`       | View/modify settings                                 |
| `/theme`          | Select color theme                                   |
| `/sandbox`        | Manage sandbox settings                              |
| `/workspace`      | Browse and switch the mounted workspace (cloud only) |
| `/add-dir <path>` | Add working directory                                |

#### Extensibility

| Command                       | Description              |
| ----------------------------- | ------------------------ |
| `/skill`, `/skills`           | Manage skills            |
| `/skill new`, `/skill create` | Create a new skill       |
| `/mcp`                        | MCP server status        |
| `/hooks`                      | View hooks configuration |
| `/commands`, `/cmds`          | Manage custom commands   |
| `/agents`                     | View subagents           |

#### Utilities

| Command                        | Description                                            |
| ------------------------------ | ------------------------------------------------------ |
| `/bashes`, `/shells`, `/tasks` | View and manage background shells                      |
| `/feedback`                    | Provide session feedback, saved locally as a .tgz file |
| `/update`                      | Update CoCo                                            |

### Session storage

| Command                                | Description            |
| -------------------------------------- | ---------------------- |
| `~/.snowflake/cortex/conversations/`   | Session files          |
| `~/.snowflake/cortex/settings.json`    | General settings       |
| `~/.snowflake/cortex/permissions.json` | Permission preferences |

See [CoCo CLI Settings](/user-guide/cortex-code/settings) for configuration details.

