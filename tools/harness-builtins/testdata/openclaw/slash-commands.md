<!-- Excerpt of https://github.com/openclaw/openclaw/blob/v2026.10.4/docs/tools/slash-commands.md -->


`/dashboard` is reserved as a built-in command. If an existing user skill is
named `dashboard`, skill discovery exposes its generated slash alias as
`/dashboard_2`. `$dashboard` and `/skill dashboard` continue to select that
user skill directly.

### Bundled plugin commands

| Command                                                                             | Description                                                                                                                                                                                    |
| ----------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `/dreaming [on\|off\|status\|help]`                                                 | Toggle memory dreaming (owner or Gateway admin). See [Dreaming](/concepts/dreaming)                                                                                                            |
| `/pair [qr\|status\|pending\|approve\|cleanup\|notify]`                             | Manage device pairing. See [Pairing](/channels/pairing)                                                                                                                                        |
| [`/voice`](/nodes/talk#choose-a-talk-voice-from-chat) `status\|list\|set <voiceId>` | Manage Talk voice config. Discord native name: `/talkvoice`                                                                                                                                    |
| `/codex <action> ...`                                                               | Bind, steer, and inspect the Codex app-server harness (status, threads, resume, model, fast, permissions, compact, review, mcp, skills, and more). See [Codex harness](/plugins/codex-harness) |

LINE-only: `/card ...` (rich card presets, see [LINE](/channels/line))

QQBot-only: `/bot-ping`, `/bot-version`, `/bot-help`, `/bot-upgrade`, `/bot-logs`

