<!-- Excerpt of https://docs.factory.com/droid-cli/cli-reference.md, fetched 2026-10-18 -->

## Droid CLI commands

| Command                              | Description                                                            | Example                                        |
| :----------------------------------- | :--------------------------------------------------------------------- | :--------------------------------------------- |
| `droid`                              | Start interactive REPL                                                 | `droid`                                        |
| `droid "query"`                      | Start REPL with initial prompt                                         | `droid "explain this project"`                 |

### Slash commands

Available when running `droid` in interactive mode. Type the command at the prompt:

| Command                       | Description                                                    |
| :---------------------------- | :------------------------------------------------------------- |
| `/cd <path>`                  | Change session working directory (alias for `/cwd`)             |
| `/cwd <path>`                 | Change session working directory                               |
| `/new`                        | Start a fresh session, reset model & autonomy to defaults      |
| `/plugins`                    | Manage plugins and marketplaces                                |
| `/quit`                       | Exit droid (alias: `exit`, or press <Kbd keys='Ctrl+C' />)     |
| `/stats [period]`             | Show usage statistics (supports relative periods, date ranges) |

