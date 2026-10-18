<!-- Excerpt of https://docs.warp.dev/agents/cli/reference.md, fetched 2026-10-18 -->

## Command-line flags

### `--help`

Show help.

## Slash commands

Type `/` at the start of the input to open the slash command menu. Commands that take an argument show a hint after the command name. Angle brackets mark required arguments, and square brackets mark optional ones.

| Command | Argument | Description |
| --- | --- | --- |
| `/agent` | `[prompt]` | Start a new conversation |
| `/api-keys` |  | View and manage model-provider API keys |
| `/clear` | `[prompt]` | Clear the transcript and start a new conversation |
| `/exit` |  | Exit the CLI |
| `/fork` | `[prompt]` | Fork the current conversation into a copy, optionally sending a prompt in the copy |
| `/new` | `[prompt]` | Start a new conversation (alias for `/agent`) |
| `/plan` | `[task]` | Ask the agent to research and create a plan for a task |
| `/theme` | `<auto|light|dark>` | Set the color theme |
| `/voice` |  | Start voice input |

Some commands appear only when they apply to your account. `/manage-billing` is listed when you administer billing for your team, and opens that team’s billing page; it isn’t available to members without billing access. `/connect-grok` opens the same subscription row as [`/api-keys`](https://docs.warp.dev/agents/cli/models-and-usage/#bring-your-own-api-key) and starts the xAI sign-in directly.

`/status` opens a read-only panel with the installed version, the current conversation’s name and ID, the working directory, and the organization and email you’re signed in as. Press `Esc` to close it, and include those details when you report a problem.

`/usage` opens a read-only panel with your plan, credit balances and consumption, and links to manage billing or upgrade. See [models and usage](https://docs.warp.dev/agents/cli/models-and-usage/#usage-and-cost).

`/team` opens a searchable list of the teams you belong to, with the active one marked. Type to filter, then press `Enter` to switch. The CLI re-scopes the current session in place, unlike the Warp app, which opens a new window for the team you pick. The active team also appears in the statusline, and clicking it opens the same list.

Skills also appear in the same menu under their own names, so you can invoke a skill directly as `/skill-name`. [Customizing the CLI](https://docs.warp.dev/agents/cli/configuration/) covers how skills are discovered.


