# Hooks, permissions and settings keys

`ai-rulez generate` can write the lifecycle hooks, permission rules and a few other settings keys of a
project into each harness's native settings file, without shipping a plugin. The keys are declared once
in `config.toml` and merged into the files through the same owned-key machinery as MCP servers (see
[Settings document merge behavior](configuration.md#settings-document-merge-behavior)): ai-rulez owns
the entries it writes, and every other key, hook group and rule in those files survives `generate` and
`clean`.

Everything on this page is opt-in. A config without `[[hooks]]`, `[permissions]` or
`[claude.settings.managed]` renders exactly what it did before.

## `[[hooks]]`

```toml
[[hooks]]
event = "PreToolUse"
matcher = "Bash"
matchers = { gemini = "run_shell_command", cursor = "Shell" }   # per-harness override
[[hooks.hooks]]
script = "scripts/guard.sh"          # or: command = "..."
timeout = 10                         # seconds
if = "Bash(git push *)"              # claude only

[[hooks]]
event = "SessionStart"
targets = ["claude", "codex"]        # optional: restrict to some harnesses
[[hooks.hooks]]
command = "ai-rulez verify"
status_message = "Checking generated files"
```

A group has the same fields as a [`[[plugin.hooks]]`](plugins.md) group (`event`, `matcher`, and `hooks`
with `command` or `script`, `args`, `timeout`, `async`, `if`, `status_message`) plus two that only exist
here, because the same declaration is rendered for several harnesses. Only `type = "command"` handlers exist here (omit `type`); `validate` and `generate` both reject other types:

- `targets`: the harnesses the group is rendered for. Empty means every harness the group can be
  expressed for. One of `claude`, `codex`, `cursor`, `gemini`, `copilot`.
- `matchers`: a matcher per harness, overriding `matcher`. Tool names differ between harnesses (Claude
  Code `Bash`, Gemini CLI `run_shell_command`), so a Claude matcher is copied only to the harnesses that
  use the same vocabulary (`claude`, `codex`). For the others a group with a `matcher` and no
  `matchers.<harness>` is skipped with a warning rather than guessed.

Events are written with Claude Code's names. Each harness has its own names and handler fields; the
renderer translates them and refuses to approximate:

| Harness | File | Shape | Timeout |
| ------- | ---- | ----- | ------- |
| `claude` | `.claude/settings.json` (`hooks`) | event, matcher group, handlers | seconds |
| `codex` | `.codex/hooks.json` (`hooks`) | event, matcher group, handlers | seconds |
| `gemini` | `.gemini/settings.json` (`hooks`) | event, matcher group, handlers | milliseconds (converted) |
| `cursor` | `.cursor/hooks.json` (`version` and `hooks`) | event, flat handler entries, camelCase events | seconds |
| `copilot` | `.github/hooks/ai-rulez.json` | event, flat handler entries, `bash`, `timeoutSec` | seconds |

Supported events per harness (the Claude Code name on the left; `-` means the harness has no equivalent
and the group is skipped with a warning):

| Claude Code event | claude | codex | gemini | cursor | copilot |
| ----------------- | ------ | ----- | ------ | ------ | ------- |
| `SessionStart` | yes | yes | `SessionStart` | `sessionStart` | `sessionStart` |
| `SessionEnd` | yes | yes | `SessionEnd` | `sessionEnd` | `sessionEnd` |
| `UserPromptSubmit` | yes | yes | `BeforeAgent` | `beforeSubmitPrompt` | `userPromptSubmitted` |
| `PreToolUse` | yes | yes | `BeforeTool` | `preToolUse` | `preToolUse` |
| `PostToolUse` | yes | yes | `AfterTool` | `postToolUse` | `postToolUse` |
| `PostToolUseFailure` | yes | - | - | `postToolUseFailure` | `postToolUseFailure` |
| `PermissionRequest` | yes | yes | - | - | `permissionRequest` |
| `Notification` | yes | - | `Notification` | - | `notification` |
| `SubagentStart` / `SubagentStop` | yes | yes | - | `subagentStart` / `subagentStop` | `subagentStart` / `subagentStop` |
| `PreCompact` | yes | yes | `PreCompress` | `preCompact` | `preCompact` |
| `PostCompact` | yes | yes | - | - | - |
| `Stop` | yes | yes | `AfterAgent` | `stop` | `agentStop` |
| every other Claude Code event | yes | - | - | - | - |

`UserPromptSubmit` and `Stop` map to Gemini's `BeforeAgent` and `AfterAgent`, the nearest events (Gemini
documents them as firing after a prompt is submitted and once per turn after the final response); they
are not identical, so review them when you target Gemini. A test compares this table with the renderer's
event tables.

Handler fields a harness lacks are never dropped silently when dropping would change behaviour:

- `if` exists in Claude Code only. A handler with `if` is skipped for every other harness, because
  running it unconditionally would widen it.
- `async` exists in Claude Code and Codex. A handler with `async = true` is skipped elsewhere.
- `args` (exec form) exists in Claude Code. For the others the arguments are shell-quoted into the
  command.
- `status_message` is cosmetic and is omitted where the harness has no equivalent.
- Copilot hooks have no matcher: a group that sets one is skipped for `copilot`.

Every skipped group is reported once per run as a warning: `[[hooks]] not generated for <harness>: ...`.
A `preset` without hook support (`opencode`, `devin`, `amp`, `junie`, `cline`, `continue-dev`,
`antigravity`, `pi`, `hermes`, `baz`, `xum`) gets one warning naming those presets. Their hook formats are
not verified against vendor documentation, so ai-rulez does not emit them.

### Scripts

`script` is a path relative to the project root. The command written to the settings file addresses it
through the variable the harness documents for its project root:

| Harness | Command for `script = "scripts/guard.sh"` |
| ------- | ----------------------------------------- |
| `claude` | `"${CLAUDE_PROJECT_DIR}"/scripts/guard.sh` |
| `gemini` | `"$GEMINI_PROJECT_DIR"/scripts/guard.sh` |
| `codex`, `copilot` | `"$(git rev-parse --show-toplevel)"/scripts/guard.sh` |
| `cursor` | `./scripts/guard.sh` (Cursor runs project hooks from the project root) |

Unlike `[[plugin.hooks]]`, the script is not copied anywhere: it is part of the repository and must be
committed, which is what makes the hook work in a fresh clone. `validate` rejects a script path that
leaves the project; `validate --strict` reports a script that does not exist (`AR504`) or lacks the
executable bit (`AR505`), and the generated `.claude/settings.json` is covered by `AR501`/`AR502`.

### Trust and review

Harnesses do not run project hooks blindly. Codex requires non-managed hooks to be reviewed and trusted
(`/hooks`) before they run and loads project hooks only for a trusted `.codex/` layer, and Claude Code
asks you to trust the folder before project `allow` rules and most `env` values apply. Generating a hook
does not bypass any of that.

## `[permissions]`

```toml
[permissions]
allow = ["Bash(git status)", "Bash(git diff *)"]
ask   = ["Bash(git push *)"]
deny  = ["Read(./.env)", "Bash(rm -rf *)"]
```

Rules use Claude Code's permission-rule syntax and are written to `permissions.allow`, `permissions.ask`
and `permissions.deny` of `.claude/settings.json`. Each rule is owned individually: rules you wrote in
the same arrays stay, a rule removed from `config.toml` is removed from the file, and a rule you wrote
that is identical to a configured one stays yours on `clean`.

`[permissions]` is generated for `claude` only. Codex approvals and sandboxing are configured through
`approval_policy`, `sandbox_mode` and rules files rather than an allow/ask/deny list in a settings file,
and no other supported harness documents such a list, so a config with `[permissions]` and another
preset prints one warning naming the presets that get nothing.

`validate` warns about an allow rule that permits every call of a tool (`Bash`, `Bash(*)`, `*`,
`WebFetch`), and `validate --strict` reports the same as `AR506 permission-overbroad`. Deny and ask
lists are not checked. ai-rulez does not evaluate rules; Claude Code applies them (deny over ask over
allow).

## `[claude.settings.managed]`

```toml
[claude.settings.managed]
env = { EXAMPLE_FLAG = "1" }
skill_overrides = { init = "off", legacy-deploy = "name-only" }
```

Keys of `.claude/settings.json` owned entry by entry: `env.<NAME>` and `skillOverrides.<skill>`.
`skill_overrides` values are `on`, `name-only`, `user-invocable-only` or `off`. Unlike the plugin keys of
`[claude.settings]`, no `manage = true` is needed: declaring an entry is the opt-in. Other entries of the
same objects survive. An entry the file already holds with the configured value is yours and stays on
`clean`; one with a different value is replaced by the configured value on `generate` (the configuration
wins, as for MCP servers) and removed on `clean`, and an entry you edit after generation stays yours.

## Files and ownership

| File | Written when | On `clean` |
| ---- | ------------ | ---------- |
| `.claude/settings.json` | MCP servers, plugin keys, `[[hooks]]`, `[permissions]` or managed keys are declared | only the entries ai-rulez wrote are removed; the file is deleted only if nothing else is left |
| `.codex/hooks.json`, `.cursor/hooks.json`, `.gemini/settings.json` | `[[hooks]]` apply to the harness | same |
| `.github/hooks/ai-rulez.json` | `[[hooks]]` apply to `copilot` | deleted (ai-rulez owns the whole file; Copilot loads every `*.json` in that directory, so hand-written hook files go beside it) |

Claims are recorded in the machine-local manifest, so `generate` also removes an entry you deleted from
`config.toml`. A document that uses comments or trailing commas (JSONC) is not rewritten; generation
fails with a hint naming the path, because the hooks were explicitly requested.

Project-wide settings are not written for `[[scopes]]` subdirectories.

## Vendor sources

Formats were read from the vendors' documentation on 2026-10-04:

- Claude Code: `code.claude.com/docs/en/hooks`, `/settings`, `/permissions`, `/skills` (skillOverrides).
- Codex: `learn.chatgpt.com/docs/hooks` (`developers.openai.com/codex/hooks` redirects there).
- Cursor: `cursor.com/docs/hooks`.
- Gemini CLI: `geminicli.com/docs/hooks/` and `/docs/hooks/reference/`.
- GitHub Copilot: `docs.github.com/en/copilot/reference/hooks-configuration`.

The event mapping for Gemini's `BeforeAgent`/`AfterAgent` is an inference from the event descriptions;
no vendor page maps Claude Code events to Gemini's.
