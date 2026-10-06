# Hooks, permissions and settings keys

`ai-rulez generate` can write the lifecycle hooks, permission rules and a few other settings keys of a
project into each harness's native settings file, without shipping a plugin. The keys are declared once
in `config.toml` and merged into the files through the same owned-key machinery as MCP servers (see
[Settings document merge behavior](configuration.md#settings-document-merge-behavior)): ai-rulez owns
the entries it writes, and every other key, hook group and rule in those files survives `generate` and
`clean`.

Everything on this page is opt-in. A config without `[[hooks]]`, `[guard]`, `[permissions]` or
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
  expressed for. One of the harnesses named in the tables below.
- `matchers`: a matcher per harness, overriding `matcher`. Tool names differ between harnesses (Claude
  Code `Bash`, Gemini CLI `run_shell_command`), so a Claude matcher is copied to the harnesses that use the
  same vocabulary (the list under the event tables) and rewritten token by token for the harnesses whose vendor
  documents its tool names (see [Tool names in matchers](#tool-names-in-matchers)). Where a token has no
  documented name, the group is skipped with a warning rather than guessed. `matchers.<harness>` always wins.

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

- `if` exists in Claude Code and Qoder. A handler with `if` is skipped for every other harness, because
  running it unconditionally would widen it.
- `async` exists in Claude Code, Codex, Qwen, Qoder, Junie and ZCode. A handler with `async = true` is skipped
  elsewhere.
- `args` (exec form) exists in Claude Code, Qoder and Deep Agents. For the others the arguments are
  shell-quoted into the command.
- `status_message` is cosmetic and is omitted where the harness has no equivalent.
- Copilot and Copilot CLI honour an optional `matcher` (a regex tested against `toolName`) on `preToolUse` and
  `postToolUse` only; on any other event a group that sets one is skipped.

Every skipped group is reported once per run as a warning: `[[hooks]] not generated for <harness>: ...`.
A preset without hook support gets one warning naming those presets; ai-rulez emits no hook it could not read
from the vendor's documentation. The harnesses that do have hook support are listed in the tables below
(settings files and plugin modules).

### Scripts

`script` is a path relative to the project root. The command written to the settings file addresses it
through the variable the harness documents for its project root:

| Harness | Command for `script = "scripts/guard.sh"` |
| ------- | ----------------------------------------- |
| `claude` | `"${CLAUDE_PROJECT_DIR}"/'scripts/guard.sh'` |
| `gemini` | `"$GEMINI_PROJECT_DIR"/'scripts/guard.sh'` |
| `codex`, `copilot` | `"$(git rev-parse --show-toplevel)"/'scripts/guard.sh'` |
| `cursor` | `'./scripts/guard.sh'` (Cursor runs project hooks from the project root) |
| every harness of the next section | see the script table there |

A script path may only contain letters, digits, `.`, `_`, `-` and `/`; `validate` rejects anything else (a
space, quote, `$`, backtick, `;`, `&`, `|`, newline or backslash would otherwise be spliced into a shell
command line). Every generated line also single-quotes the path, and a user-scope config directory is
single-quoted with embedded quotes escaped, so neither can inject a command. A `command` is yours and is
copied verbatim.

Unlike `[[plugin.hooks]]`, the script is not copied anywhere: it is part of the repository and must be
committed, which is what makes the hook work in a fresh clone. `validate` rejects a script path that
leaves the project; `validate --strict` reports a script that does not exist (`AR504`) or lacks the
executable bit (`AR505`), and the generated `.claude/settings.json` is covered by `AR501`/`AR502`.

### More settings-file harnesses

The harnesses below share one renderer driven by a table of vendor facts: the events a vendor documents, the
handler field names, the timeout unit, whether a matcher is honoured and on which events. The output is a
hook group merged into the file like the five above (hand-written hooks stay, `clean` takes back only what was
written), and `generate --user` writes the user-level file. `Layout` says how the document is built.

| Harness | Project file | User file (`--user`) | Layout | Timeout |
| ------- | ------------ | -------------------- | ------ | ------- |
| `copilot-cli` | `.github/hooks/ai-rulez.json` (the file `copilot` writes; both render it identically) | `~/.copilot/hooks/ai-rulez.json` | `version` and `hooks`, flat entries with `bash`, `timeoutSec` | seconds |
| `factory` | `.factory/hooks.json` | `~/.factory/hooks.json` | events keyed at the document root, matcher groups | seconds |
| `antigravity` | `.agents/hooks.json` | `~/.gemini/config/hooks.json` | named hook groups; ai-rulez owns the group `ai-rulez` | seconds |
| `qwen` | `.qwen/settings.json` (`hooks`) | `~/.qwen/settings.json` | matcher groups | seconds |
| `augment` | `.augment/settings.json` (`hooks`) | `~/.augment/settings.json` | matcher groups | milliseconds (converted) |
| `codebuddy` | `.codebuddy/settings.json` (`hooks`) | `~/.codebuddy/settings.json` | matcher groups | seconds |
| `qoder` | `.qoder/settings.json` (`hooks`) | `~/.qoder/settings.json` | matcher groups, exec-form `args` | seconds |
| `commandcode` | `.commandcode/settings.json` (`hooks`) | `~/.commandcode/settings.json` | matcher groups | seconds |
| `letta` | `.letta/settings.json` (`hooks`) | `~/.letta/settings.json` | matcher groups | milliseconds (converted) |
| `gitlab-duo` | `.gitlab/duo/hooks.json` (`hooks`) | `~/.gitlab/duo/hooks.json` | matcher groups | seconds |
| `devin` | `.devin/hooks.v1.json` | `hooks` of `~/.config/devin/config.json` | events keyed at the document root, matcher groups | seconds |
| `grok` | `.grok/hooks/ai-rulez.json` | `~/.grok/hooks/ai-rulez.json` | `hooks`, matcher groups | seconds |
| `bob` | `.bob/settings.json` (`hooks`) | `~/.bob/settings/settings.json` | matcher groups | seconds |
| `cortex` | `.cortex/settings.json` (`hooks`) | `~/.snowflake/cortex/hooks.json` | matcher groups | seconds |
| `goose` | `.agents/plugins/ai-rulez/hooks/hooks.json` | `~/.agents/plugins/ai-rulez/hooks/hooks.json` | `hooks`, matcher groups (a hook-only plugin directory) | seconds |
| `deepagents` | `.deepagents/hooks.json` | `~/.deepagents/hooks.json` (`DEEPAGENTS_HOME`) | `hooks`, matcher groups, exec-form `argv` | seconds |
| `junie` | none: Junie ignores project hooks | `~/.junie/config.json` (`hooks`) | matcher groups | seconds |
| `zcode` | none: ZCode ignores project hooks | `~/.zcode/cli/config.json` (`hooks.events`, and `hooks.enabled`) | matcher groups, `timeoutMs` | milliseconds (converted) |
| `crush` | `crush.json` (`hooks`) | `~/.config/crush/crush.json` | one flat list of entries per event | seconds |
| `poolside` | `.poolside/settings.yaml` (`hooks`) | `~/.config/poolside/settings.yaml` | YAML, one flat list per event, every entry has a `matcher` (`*` by default) | seconds |
| `reasonix` | `.reasonix/settings.json` (`hooks`) | `~/.reasonix/settings.json` | one flat list per event, `match` | milliseconds (converted) |
| `hermes` | none: no project hooks documented | `~/.hermes/config.yaml` (`hooks`, `HERMES_HOME`) | YAML, one flat list per event | seconds |
| `kiro` | `.kiro/hooks/ai-rulez.json` | `~/.kiro/hooks/ai-rulez.json` | `version: "v1"` and one flat `hooks` list, each entry has its `trigger` and an `action` | seconds |
| `vibe` | `.vibe/hooks.toml` | `~/.vibe/hooks.toml` (`VIBE_HOME`) | TOML, one flat `[[hooks]]` list, the event in `type`, the tool glob in `match` | seconds |
| `kimi` | none: no project hooks documented | `~/.kimi-code/config.toml` (`[[hooks]]`, `KIMI_CODE_HOME`) | TOML, one flat `[[hooks]]` list, the event in `event` | seconds |
| `cline` | `.clinerules/hooks/<Event>` | `~/Documents/Cline/Hooks/<Event>` | one executable script per event | n/a |

Supported events per harness, by Claude Code name; a native name in parentheses is shown where it differs.
An event not listed is skipped for that harness with a warning, and so is a matcher on an event the vendor
says ignores one:

| Harness | Events |
| ------- | ------ |
| `copilot-cli` | `SessionStart` (`sessionStart`), `UserPromptSubmit` (`userPromptSubmitted`), `PreToolUse` (`preToolUse`), `PermissionRequest` (`permissionRequest`), `PostToolUse` (`postToolUse`), `PostToolUseFailure` (`postToolUseFailure`), `Notification` (`notification`), `SubagentStart` (`subagentStart`), `SubagentStop` (`subagentStop`), `Stop` (`agentStop`), `PreCompact` (`preCompact`), `SessionEnd` (`sessionEnd`) |
| `factory` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Notification`, `SubagentStop`, `Stop`, `PreCompact`, `SessionEnd` |
| `antigravity` | `PreToolUse`, `PostToolUse`, `Stop` |
| `qwen` | `SessionStart`, `UserPromptSubmit`, `UserPromptExpansion`, `PreToolUse`, `PermissionRequest`, `PermissionDenied`, `PostToolUse`, `PostToolUseFailure`, `PostToolBatch`, `Notification`, `MessageDisplay`, `SubagentStart`, `SubagentStop`, `Stop`, `StopFailure`, `InstructionsLoaded`, `PreCompact`, `PostCompact`, `SessionEnd` |
| `augment` | `SessionStart`, `PreToolUse`, `PostToolUse`, `Stop`, `SessionEnd` |
| `codebuddy` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PermissionDenied`, `PostToolUse`, `PostToolUseFailure`, `Notification`, `SubagentStart`, `SubagentStop`, `TaskCreated`, `TaskCompleted`, `Stop`, `StopFailure`, `InstructionsLoaded`, `ConfigChange`, `CwdChanged`, `FileChanged`, `WorktreeCreate`, `WorktreeRemove`, `PreCompact`, `PostCompact`, `Elicitation`, `ElicitationResult`, `SessionEnd` |
| `qoder` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PermissionDenied`, `PostToolUse`, `PostToolUseFailure`, `Notification`, `SubagentStart`, `SubagentStop`, `Stop`, `StopFailure`, `InstructionsLoaded`, `ConfigChange`, `CwdChanged`, `FileChanged`, `WorktreeCreate`, `WorktreeRemove`, `PreCompact`, `PostCompact`, `Elicitation`, `ElicitationResult`, `SessionEnd` |
| `commandcode` | `SessionStart`, `PreToolUse`, `PostToolUse`, `Stop` |
| `letta` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PostToolUseFailure`, `Notification`, `SubagentStop`, `Stop`, `PreCompact`, `SessionEnd` |
| `gitlab-duo` | `SessionStart` |
| `devin` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `Stop`, `PostCompact` (`PostCompaction`), `SessionEnd` |
| `grok` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionDenied`, `PostToolUse`, `PostToolUseFailure`, `Notification`, `SubagentStart`, `SubagentStop`, `Stop`, `StopFailure`, `PreCompact`, `PostCompact`, `SessionEnd` |
| `bob` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop` |
| `cortex` | `SessionStart`, `Setup`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `Notification`, `SubagentStop`, `Stop`, `PreCompact`, `SessionEnd` |
| `goose` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `Stop`, `SessionEnd` |
| `deepagents` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `Notification`, `SubagentStart`, `SubagentStop`, `Stop`, `PreCompact`, `SessionEnd` |
| `junie` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `Stop`, `StopFailure`, `SessionEnd` |
| `zcode` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PostToolUseFailure`, `Stop` |
| `crush` | `PreToolUse` |
| `poolside` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `PreCompact` |
| `reasonix` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PostToolUseFailure`, `Notification`, `SubagentStop`, `Stop`, `StopFailure`, `PreCompact`, `SessionEnd` |
| `hermes` | `SessionStart` (`on_session_start`), `PreToolUse` (`pre_tool_call`), `PostToolUse` (`post_tool_call`), `SubagentStop` (`subagent_stop`), `SessionEnd` (`on_session_end`) |
| `kiro` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop` |
| `vibe` | `PreToolUse` (`pre_tool`), `PostToolUse` (`post_tool`), `Stop` (`post_agent`) |
| `kimi` | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PostToolUseFailure`, `Notification`, `SubagentStart`, `SubagentStop`, `Stop`, `StopFailure`, `PreCompact`, `PostCompact`, `SessionEnd` |
| `cline` | `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PreCompact`, `SessionStart` (`TaskStart`), `Stop` (`TaskComplete`) |

A test compares this table with the renderer's event tables. Mappings that are an inference, not something the
vendor documents: `Stop` to Vibe's `post_agent`, to Cline's `TaskComplete` and to Copilot CLI's `agentStop`;
`SessionStart` to Cline's `TaskStart`. Review them when you target those harnesses.

Matchers follow what each vendor documents. Claude Code names (`Bash`, `Edit|Write`) are copied to the
harnesses that accept them: `claude`, `codex`, `qwen`, `codebuddy`, `qoder`, `letta`, `grok`, `deepagents`,
`junie`, `zcode`. For the harnesses in the next section the matcher is rewritten; for every other one
(`commandcode`, `bob`, `reasonix`, `hermes`, `kimi`, ...) a group with a `matcher` needs `matchers.<harness>`.
Two matchers are not tool names at all: Duo CLI's matches the session source (`startup` or `resume`) and Vibe's
`match` is a glob (or a regex behind `re:`). Where a vendor requires a matcher field Poolside and Vibe get `*`
when the group sets none.

### Tool names in matchers

A matcher is rewritten token by token over its top-level alternation (`Edit|Write`, `^Bash$`,
`mcp__github__.*`) through the vocabulary in `internal/toolnames`, the one table that hook matchers, the
plugin runtimes (`opencode`, `kilo`, `pi`, `amp`) and this page share. A Claude name outside the harness's
vocabulary, a regular expression more complex than a name, or an alternation in a harness whose matcher is a
glob or an exact name skips the whole group with a warning naming the token; a translation is never partial.
Harnesses that test the matcher as an unanchored regular expression get each name anchored (`^Execute$`), since a
Claude matcher is a whole-name match. Names below were read from the cited pages on 2026-10-05; a tool a vendor
does not document is left out.

| Harness | Claude tool to native name | MCP tools | Source |
| --- | --- | --- | --- |
| `cursor` | Bash `Shell`; Read `Read`; Edit, MultiEdit, Write `Write` (Cursor files edits under Write); Grep `Grep`; Task, Agent `Task` | not rewritten (`MCP:<tool>` carries no server) | cursor.com/docs/hooks |
| `gemini` | Bash `run_shell_command`; Read `read_file`, `read_many_files`; Edit, MultiEdit `replace`; Write `write_file`; Grep `grep_search`; Glob `glob`; LS `list_directory`; WebFetch `web_fetch`; WebSearch `google_web_search`; TodoWrite `write_todos` | `mcp_<server>_<tool>` | geminicli.com/docs/reference/tools/ |
| `copilot`, `copilot-cli` | Bash `bash`; Read `view`; Edit, MultiEdit `edit`; Write `create`; Grep `grep`; Glob `glob`; WebFetch `web_fetch`; Task, Agent `task` | not documented | docs.github.com/en/copilot/reference/hooks-configuration |
| `factory` | Bash `Execute`; Read `Read`; Edit `Edit`; Write `Create`; Grep `Grep`; Glob `Glob`; LS `LS`; WebFetch `FetchUrl`; WebSearch `WebSearch`; Task, Agent `Task` | `mcp__<server>__<tool>` | docs.factory.com/reference/hooks-reference |
| `devin` | Bash `exec`; Read `read`; Edit `edit`; Write `write`; Grep `grep`; Glob `glob`; WebFetch `webfetch` | `mcp__<server>__<tool>` | docs.devin.ai/cli/extensibility/hooks/lifecycle-hooks |
| `kiro` | Bash `shell` (one tool, no alternation; Kiro's other categories span several Claude tools) | not rewritten | kiro.dev/docs/hooks/types/ |
| `goose` | Bash `shell`; Write `write`; Edit, MultiEdit `edit` | `<server>__<tool>` | goose-docs.ai/docs/guides/context-engineering/hooks/ |
| `crush` | Bash `bash`; Read `view`; Edit `edit`; Write `write`; MultiEdit `multiedit`; Grep `grep`; Glob `glob`; LS `ls`; Task, Agent `agent` | `mcp_<server>_<tool>` | github.com/charmbracelet/crush docs/hooks/README.md |
| `cortex` | Bash `bash`; Read `read`; Edit `edit`; Write `write`; Grep `grep`; Glob `glob`; NotebookEdit `notebook_edit_cell` | `mcp__<server>__<tool>` | docs.snowflake.com/en/user-guide/cortex-code/extensibility |
| `poolside` | Bash `shell`; Read `read`; Edit, MultiEdit `edit`; Write `write`; WebFetch `web_fetch`; WebSearch `web_search` | not documented | docs.poolside.ai/hooks |
| `vibe` | Bash `bash`; Grep `grep` (a glob: one tool, no alternation) | not rewritten | docs.mistral.ai/vibe/code/cli/hooks |
| `augment` | Bash `launch-process`; Read `view`; Edit, MultiEdit `str-replace-editor`; Write `save-file`; WebFetch `web-fetch`; WebSearch `web-search` | not rewritten (`mcp:` prefix, `<tool>_<server>`) | docs.augmentcode.com/cli/hooks |
| `antigravity` | Bash `run_command`; Read `view_file`; Edit `replace_file_content`; MultiEdit `multi_replace_file_content`; Write `write_to_file`; Grep `grep_search`; Glob `find_by_name`; LS `list_dir`; WebFetch `read_url_content`; WebSearch `search_web`; Task, Agent `invoke_subagent` | not documented | antigravity.google/docs/hooks |

Left unmapped on purpose: Command Code (the page names the tools in two cases, `SHELL` and `shell`), Bob (only
`write_file` is documented) and Reasonix (no `match` documentation could be read). Cursor's `beforeShellExecution`
is not used for `PreToolUse` with `Bash`: its matcher is tested against the command text, not the tool, so it
is not equivalent; the `preToolUse` matcher `Shell` is. Copilot hooks carry the tool name on stdin as well, but
no sh filter is generated: the documented `matcher` makes one unnecessary.

Handlers are `command` handlers everywhere; a handler of another `type`, and a handler field the harness lacks
(`if`, `async`), is skipped with a warning like for the harnesses above. `script` is addressed through the
variable the vendor documents:

| Harness | Command for `script = "scripts/guard.sh"` |
| ------- | ----------------------------------------- |
| `qwen` | `"$QWEN_PROJECT_DIR"/scripts/guard.sh` |
| `codebuddy` | `"$CODEBUDDY_PROJECT_DIR"/scripts/guard.sh` |
| `qoder` | `"$QODER_PROJECT_DIR"/scripts/guard.sh` |
| `commandcode` | `"$COMMANDCODE_PROJECT_DIR"/scripts/guard.sh` |
| `factory` | `"$FACTORY_PROJECT_DIR"/scripts/guard.sh` |
| `devin` | `"$DEVIN_PROJECT_DIR"/scripts/guard.sh` |
| `cortex` | `"$CORTEX_PROJECT_DIR"/scripts/guard.sh` |
| `gitlab-duo` | `"$DUO_PROJECT_DIR"/scripts/guard.sh` |
| `grok` | `"$GROK_WORKSPACE_ROOT"/scripts/guard.sh` |
| `deepagents` | `"${CLAUDE_PROJECT_DIR}"/scripts/guard.sh` |
| `letta`, `antigravity`, `bob`, `kiro`, `vibe` | `./scripts/guard.sh` (the vendor's examples run project scripts relative to the project) |
| `cline` | `"$root"/scripts/guard.sh`, where the wrapper computes `$root` from its own location |
| `crush` | `"$CRUSH_PROJECT_DIR"/scripts/guard.sh` |
| `augment`, `goose`, `poolside`, `reasonix` | skipped with a warning in project scope (no documented way to address the project); use `command` |

In user scope every `script` is the absolute path in the user config directory.

Not generated, and why:

- `junie`, `zcode`, `hermes` and `kimi` have no project-level hooks (Junie and ZCode ignore them explicitly), so a
  project `generate` skips them with a warning and `generate --user` writes them.
- `codewhale`: its project hooks file loads only after the user approves a digest of its exact bytes, and
  its events do not map to Claude Code's without guessing.
- `continue-dev`: the preset no longer exists, and its hook format is documented only in source code.
- Copilot CLI's `preMcpToolCall` and Augment's `PromptSubmit` appear in third-party mappings but in no vendor
  documentation, so they are not emitted.

Some harnesses need a step from you before a generated project hook runs, which generation cannot do: Duo CLI
needs `--enable-project-hooks` (or `GITLAB_ENABLE_PROJECT_HOOKS=true`), Grok needs `/hooks-trust`, Deep Agents
asks to trust the file. Devin also loads the hooks of `.claude/settings.json`, so a
hook generated for both `claude` and `devin` runs twice. Each of these is printed once per run.

#### Cline

Cline has no hooks document: it runs the executable named after an event from `.clinerules/hooks/` (and from
`~/Documents/Cline/Hooks/`). `generate` writes one executable POSIX shell script per event
(`PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `PreCompact`, `TaskStart`, `TaskComplete`). Each runs the
configured commands in order with the hook input on stdin (read up to 16 MiB); each command runs inside a
`{ ...; }` group, so `cd dir && ./check` or a trailing `# comment` behave as written and its exit status is
the group's. The last command's output is the hook's output and the first failing command stops the rest. Cline has no matcher, timeout or `async`, so a group or handler that
sets one is skipped with a warning. The scripts are owned whole, gitignored and removed by `clean`; a script
is treated as generated only when its second line (after the shebang) starts with `# Generated by ai-rulez`,
so a script you wrote under the same name, even one that mentions the marker in a comment, is never
overwritten (a warning says so). Cline on Windows expects `.ps1` files,
which are not generated. The vendor's current documentation describes only SDK plugins; the file contract was
read from Cline's source (see Vendor sources).

### Plugin-based harnesses

OpenCode, Kilo, MiMo Code, Pi and Amp have no hooks file: their hooks are code, loaded from a plugin or
extension directory. `[[hooks]]` render for them (`targets` and `matchers` accept `opencode`, `kilo`,
`mimocode`, `pi` and `amp`) into one JavaScript or TypeScript module that ai-rulez owns whole. It is
gitignored (the module only, never its directory, which also holds hand-written plugins), removed by
`clean` and by dropping the last hook, and written to the user-level directory by `generate --user`.

| Harness | Project file | User file (`--user`) | Plugin API |
| ------- | ------------ | -------------------- | ---------- |
| `opencode` | `.opencode/plugins/ai-rulez-hooks.js` | `~/.config/opencode/plugins/ai-rulez-hooks.js` | default export with the v2 `setup(ctx)` and the v1 `server(ctx)`, so it loads on either generation |
| `kilo` | `.kilo/plugins/ai-rulez-hooks.js` | `~/.config/kilo/plugins/ai-rulez-hooks.js` | default export `{ id, server }` |
| `mimocode` | `.mimocode/plugins/ai-rulez-hooks.js` | `~/.config/mimocode/plugins/ai-rulez-hooks.js` | default export `{ id, server }` |
| `pi` | `.pi/extensions/ai-rulez-hooks.ts` | `~/.pi/agent/extensions/ai-rulez-hooks.ts` | default-exported factory, `pi.on(event, handler)` |
| `amp` | `.amp/plugins/ai-rulez-hooks.ts` | `~/.config/amp/plugins/ai-rulez-hooks.ts` | default-exported function, `amp.on(event, handler)` |

Events (Claude Code name, then the native event):

| Claude Code event | opencode, kilo, mimocode | pi | amp |
| ----------------- | ------------------------ | -- | --- |
| `SessionStart` | `session.created` | `session_start` | `session.start` |
| `SessionEnd` | - | `session_shutdown` | - |
| `UserPromptSubmit` | - | `input` (can cancel the prompt) | `agent.start` |
| `PreToolUse` | `tool.execute.before` (can block) | `tool_call` (can block) | `tool.call` (can block) |
| `PostToolUse` | `tool.execute.after` | `tool_result`, when the call succeeded | `tool.result`, status `done` |
| `PostToolUseFailure` | - | `tool_result`, when `isError` | `tool.result`, status `error` |
| `PreCompact` / `PostCompact` | - / `session.compacted` | `session_before_compact` / `session_compact` | - |
| `Stop` | `session.idle` | `agent_settled` | `agent.end` |

An event outside the column is skipped with a warning. OpenCode reports `session.created` and
`session.idle` for subagent sessions too, so those hooks run for them as well; Pi's `agent_settled`
fires once when a run is final.

What the generated module does for every command:

- The command runs in a shell from the project directory with a Claude Code style JSON document on stdin:
  `session_id`, `cwd`, `hook_event_name` and the fields of the event (`tool_name`, `tool_input`,
  `tool_response`, `prompt`, `source`, `trigger`, `reason`). The harness defines no hook payload of its
  own. `tool_name` is the Claude Code name where one is mapped (`bash` becomes `Bash`, `edit_file`
  becomes `Edit`) and `harness_tool_name` is the harness's own name; `tool_input` also carries `command`
  and `file_path` as Claude Code names them. `CLAUDE_PROJECT_DIR` and `AI_RULEZ_PROJECT_DIR` hold the
  project directory.
- A `matcher` is a regular expression that must match a whole tool name, and it is tested against the
  harness's name and the Claude Code names mapped to it, so `Edit|Write` selects OpenCode's `edit`,
  `write` and `apply_patch` and Pi's `edit` and `write`. A name outside the table (an MCP tool) matches only
  by its own name, so write it in `matchers.<harness>`. On `SessionStart` the matcher is tested against the
  source (`startup`, and for Pi `resume` and `clear`), on `PreCompact` and `PostCompact` against `manual`
  or `auto` (Pi). On any other event without a subject a matcher skips the group with a warning.
- Exit code 2 from a `PreToolUse` hook blocks the call and its stderr becomes the reason (Pi also blocks a
  prompt on `UserPromptSubmit`); so does a JSON decision of `block` or `deny` on stdout. Any other non-zero
  exit, a timeout (`timeout`, default 600 seconds) or a spawn failure is logged to stderr and never blocks.
  Hooks of other events cannot block. `async = true` runs the command without waiting for it.
- A handler with `if` or a non-command `type` is skipped with a warning, as for the other harnesses.
- A `script` runs as `./<script>` from the project directory (an absolute path in the user config
  directory for `--user`); `args` are shell-quoted into the command.

The hook commands are embedded as one JSON literal produced by the Go encoder, never spliced into code,
so quotes, newlines, backticks and `${}` in a command reach the shell as written.

A module is owned only while its first line is the `// Generated by ai-rulez` banner. A hand-written plugin
with the same file name is never overwritten: it is left alone and a warning says so.

A `matcher` is checked when the module is generated. It must be a regular expression that Go and
JavaScript read alike: unbalanced groups (`a)|(b`, which would escape the `^(?:...)$` anchoring), inline
flags, named groups, lookarounds, backreferences, POSIX classes, `\p{..}` and `\z` are rejected, and the
group is skipped with a warning instead of generating a matcher that fails to compile and matches nothing
(which would silently disable a blocking hook).

Amp requires a `tool.call` handler to return a result, and `{ action: "allow" }` is the only one that
neither rejects nor rewrites the call; Amp documents no way to return "no opinion", so a non-blocking hook
returns it. Whether Amp's own permission rules still apply after `allow` is not documented.

### Environment and failure semantics

- A hook command inherits the full environment of the harness process, secrets included (tokens, cloud
  credentials), and runs with your privileges. ai-rulez does not filter it: a `command` or `script` in
  `[[hooks]]` has the same access as you do, so review them like code. The plugin modules add
  `CLAUDE_PROJECT_DIR`, `AI_RULEZ_PROJECT_DIR` and `AI_RULEZ_HOOK_EVENT`.
- Hooks fail open. As in Claude Code, only a deliberate block (exit code 2, or a JSON `block`/`deny`
  decision) stops an event; any other non-zero exit, a timeout, a missing script or a spawn failure is
  logged and the event proceeds. A guard that must not be bypassed has to exit 2 itself on every error
  path (`set -e` is not enough, it exits 1), and a deny that matters belongs in `[permissions]` or a sandbox
  as well. There is no `fail_closed` option: Claude Code has no such model, and emulating it per harness
  would change what a hook error means from harness to harness.
- Harnesses that merge several hook sources run them all, so a hook generated for two overlapping
  harnesses (Devin and Claude, for example) can run twice.

### Trust and review

Harnesses do not run project hooks blindly. Codex requires non-managed hooks to be reviewed and trusted
(`/hooks`) before they run and loads project hooks only for a trusted `.codex/` layer, and Claude Code
asks you to trust the folder before project `allow` rules and most `env` values apply. Generating a hook
does not bypass any of that.

## `[guard]`

```toml
[guard]
generated = true
# version = "5.0.0"          # npx version to run; default is the generating binary's version
# command = ["ai-rulez"]     # replaces `npx -y ai-rulez@<version>`; excludes version
```

Agents ignore the "do not edit" banner of a generated file often enough that the edit is lost on the next
`generate`. With `generated = true`, `generate` adds a `PreToolUse` hook that runs `ai-rulez guard` before
every file-editing tool call and blocks the ones that target a generated file. The agent is told to edit the
source under `.ai-rulez/` (named from the file's `Generated by ai-rulez from <source>` banner when it has
one) and to read the ai-rulez skill.

It is a table of its own because `[hooks]` cannot coexist with the `[[hooks]]` array of tables in TOML. The
hook is synthesized at generation time and written like any other `[[hooks]]` group; it is never saved back to
`config.toml`.

| Harness | Event | Matcher it renders |
| ------- | ----- | ------------------ |
| `claude` | `PreToolUse` | `Edit\|Write\|MultiEdit` |
| `codex` | `PreToolUse` | `apply_patch\|Edit\|Write` |
| `gemini` | `BeforeTool` | `^replace$\|^write_file$` |
| `cursor` | `preToolUse` | `^Write$` |
| `factory` | `PreToolUse` | `Edit\|Create` |

Only harnesses whose documented `PreToolUse` hook blocks a call on exit code 2 get it; the others are skipped
and `generate` logs which at info level. The hook command resolves the executable the way
[`[mcp] self_server`](configuration.md) does, so it works through `npx` without a global install.

How `ai-rulez guard` decides:

- It reads the hook JSON on stdin (`tool_name`, `tool_input.file_path`, `path`, `target_file`, or the
  `*** Update File:` lines of a Codex `apply_patch`) and resolves the path against the payload's `cwd`.
  `..` segments and symlinks are resolved first, so neither reaches a generated file by another name.
- A path is blocked only when it is a wholly owned output listed in `.ai-rulez/.generated-manifest.json` or
  `.generated-manifest.local.json`. Documents ai-rulez only merges keys into (`.claude/settings.json`,
  `.vscode/settings.json`, `.mcp.json`, ...) are never blocked. Read-only tools are never blocked.
- It reads the two manifests and nothing else: no config load, no network, a few milliseconds.
- It fails open on its own errors: a payload that does not parse, a project without a manifest and a path
  outside the project all exit 0. A blocked call exits 2 with the reason on stderr. A guard that errors must
  not wedge the agent, and [`ai-rulez verify`](cli.md#verify-command) still catches a hand edit afterwards.
- It does not fail open on input it will not analyse: a payload over 8 MiB, or a call naming more than 1000
  distinct files, exits 2 with a message asking for smaller edits. Padding a patch is not a bypass. Repeated
  paths are collapsed, and the project and manifests are looked up once per call, so a large patch stays well
  inside a hook timeout.

The guard stops an agent's file-editing tools. Shell-based writes (`sed -i`, `tee`, a `>` redirect or a script
run through a Bash tool) are not blocked; a deny that must hold belongs in `[permissions]` or a sandbox as well.

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

Besides Claude Code, `[permissions]` is translated into the native permission format of every harness that
has one (Codex rules, OpenCode `permission`, Cursor `cli.json`, VS Code `chat.tools.*` and others). See
[Permissions](permissions.md) for the per-harness table, what is lossy and the sources. A config with
`[permissions]` and a preset that has no documented permission file prints one warning naming the
presets that get nothing, and says that their deny rules are not enforced.

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

A [role](roles.md)'s `skill_mode` is rendered through the same ownership: `generate --role <name>` owns
`skillOverrides.<skill>` for the skills the role sets, the role's value wins over the same skill in
`skill_overrides`, and switching roles removes the previous role's entries.

## Files and ownership

| File | Written when | On `clean` |
| ---- | ------------ | ---------- |
| `.claude/settings.json` | plugin keys, `[[hooks]]`, `[permissions]` or managed keys are declared (MCP servers are not: Claude Code reads them from `.mcp.json`) | only the entries ai-rulez wrote are removed; the file is deleted only if nothing else is left |
| `.codex/hooks.json`, `.cursor/hooks.json`, `.gemini/settings.json` | `[[hooks]]` apply to the harness | same |
| `.opencode/plugins/ai-rulez-hooks.js`, `.kilo/plugins/ai-rulez-hooks.js`, `.mimocode/plugins/ai-rulez-hooks.js`, `.pi/extensions/ai-rulez-hooks.ts`, `.amp/plugins/ai-rulez-hooks.ts` | `[[hooks]]` apply to the harness | deleted (ai-rulez owns the whole module; the directory also holds hand-written plugins, which are left alone) |
| `.factory/hooks.json`, `.agents/hooks.json`, `.devin/hooks.v1.json`, `.gitlab/duo/hooks.json`, `.grok/hooks/ai-rulez.json`, `.kiro/hooks/ai-rulez.json`, `.deepagents/hooks.json`, `.agents/plugins/ai-rulez/hooks/hooks.json`, `.vibe/hooks.toml` | `[[hooks]]` apply to the harness | only the hooks ai-rulez wrote are removed; the file goes when nothing else is in it |
| `.qwen/settings.json`, `.augment/settings.json`, `.codebuddy/settings.json`, `.qoder/settings.json`, `.commandcode/settings.json`, `.letta/settings.json`, `.bob/settings.json`, `.cortex/settings.json`, `.reasonix/settings.json`, `crush.json`, `.poolside/settings.yaml` | MCP servers or `[[hooks]]` apply to the harness | same; MCP servers and hooks of one file are merged together |
| user-level `~/.junie/config.json`, `~/.zcode/cli/config.json` (`hooks.events`, `hooks.enabled`), `~/.hermes/config.yaml`, `~/.kimi-code/config.toml` | `generate --user` with `[[hooks]]` | same; ZCode's `hooks.enabled` is added only when absent and kept on `clean` while the file holds other settings |
| `.clinerules/hooks/<Event>` | `[[hooks]]` apply to `cline` | deleted (ai-rulez owns the script; one without its `Generated by ai-rulez` line is yours and is never overwritten) |
| `.github/hooks/ai-rulez.json` | `[[hooks]]` apply to `copilot` or `copilot-cli` | deleted (ai-rulez owns the whole file; Copilot loads every `*.json` in that directory, so hand-written hook files go beside it) |

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

- OpenCode: `opencode.ai/docs/plugins/` and, for the v2 plugin API, `opencode.ai/v2/docs/build/plugins` (the
  v1 `server()` plus v2 `setup()` default export is documented under "Support V1"); plugin directories from
  `packages/opencode/src/config/plugin.ts` (`{plugin,plugins}/*.{ts,js}`).
- Kilo: `kilo.ai/docs/automate/extending/plugins` and `Kilo-Org/kilocode` (`packages/opencode/src/config/plugin.ts`).
- MiMo Code: `XiaomiMiMo/MiMo-Code` (`packages/cli/src/config/plugin.ts`, `packages/cli/src/plugin/index.ts`).
- Pi: `earendil-works/pi`, `packages/coding-agent/docs/extensions.md` and
  `src/core/extensions/types.ts` (event payloads, `ToolCallEventResult`).
- Amp: `ampcode.com/docs/plugin-api`, `ampcode.com/docs/customize/plugins` and the `@ampcode/plugin` typings.
  Amp's tool names beyond `Bash`, `Read`, `Grep` and `Task` (`edit_file`, `create_file`, `glob`, `web_search`,
  `read_web_page`, `todo_write`) are not listed on any documentation page; they are from the tool names
  Amp uses elsewhere, so check them with `amp tools list` and use `matchers.amp` if one differs.

Settings-file harnesses of "More settings-file harnesses", read 2026-10-05 (the vendor's page, and for Cline
and Continue its source, where no page describes the file format):

- Copilot CLI: `docs.github.com/en/copilot/reference/hooks-configuration`.
- Factory Droid: `docs.factory.com/reference/hooks-reference` and `/cli/configuration/hooks-guide`.
- Antigravity: `antigravity.google/docs/hooks`.
- Qwen Code: `qwenlm.github.io/qwen-code-docs/en/users/features/hooks/`. The user-level path is not stated
  there; `~/.qwen/settings.json` is the location Qwen Code's other user settings use.
- Auggie (Augment): `docs.augmentcode.com/cli/hooks`.
- CodeBuddy Code: `codebuddy.ai/docs/cli/hooks`.
- Qoder CLI: `docs.qoder.com/cli/hooks`.
- Command Code: `commandcode.ai/docs/hooks`.
- Letta Code: `docs.letta.com/letta-code/hooks`.
- GitLab Duo CLI: `docs.gitlab.com/user/gitlab_duo_cli/customize/` (hooks are an experiment).
- Devin CLI: `docs.devin.ai/cli/extensibility/hooks/overview`.
- Grok Build CLI: `docs.x.ai/build/features/hooks`.
- IBM Bob: `bob.ibm.com/docs/ide/configuration/lifecycle-hooks`.
- Snowflake Cortex Code: `docs.snowflake.com/en/user-guide/cortex-code/extensibility`. A second page names
  `.snowflake/cortex/settings.json` as the project file; `.cortex/settings.json` is the one the CLI page and the
  preset use.
- Block goose: `goose-docs.ai/docs/guides/context-engineering/hooks/`.
- Deep Agents Code: `docs.langchain.com/oss/deepagents/code/hooks`.
- Junie CLI: `junie.jetbrains.com/docs/junie-cli-hooks.html`.
- ZCode: `zcode.z.ai/en/docs/hooks`.
- Crush: `github.com/charmbracelet/crush`, `docs/hooks/README.md`. Crush prefers a `crushrc` script but still
  reads the `hooks` of `crush.json`.
- Poolside Pool: `docs.poolside.ai/hooks`.
- Reasonix: `DeepSeek-Reasonix`, `docs/DESKTOP_HOOKS.zh-CN.md` and `internal/hook/hook.go` (the project's
  documentation is a desktop guide).
- Hermes Agent: `hermes-agent.nousresearch.com/docs/user-guide/features/hooks`.
- Kiro: `kiro.dev/docs/hooks/` (the standalone `.kiro/hooks/*.json` format of the IDE 1.0 and CLI 3.0); the
  user-level `~/.kiro/hooks/` directory is from the vendor's changelog, and the file name `ai-rulez.json`
  there is ours.
- Mistral Vibe: `docs.mistral.ai/vibe/code/cli/hooks`.
- Kimi Code: `moonshotai.github.io/kimi-code/en/customization/hooks.html` (the older kimi-cli documents
  `~/.kimi/config.toml`).
- Cline: the extension and SDK sources `apps/vscode/src/core/hooks/utils.ts` and
  `sdk/packages/core/src/hooks/hook-file-config.ts` of `github.com/cline/cline`; docs.cline.bot only describes
  SDK plugins now.

The event mapping for Gemini's `BeforeAgent`/`AfterAgent` is an inference from the event descriptions;
no vendor page maps Claude Code events to Gemini's.
