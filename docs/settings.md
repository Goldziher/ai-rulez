# Hooks and settings

`ai-rulez generate` writes lifecycle hooks, the generated-file guard and a few other settings keys into each
harness's native settings file, from one declaration in `config.toml`. ai-rulez owns only the entries it writes:
every other key, hook group and rule in those files survives `generate` and `clean`. Permission rules have a page of
their own: [Permissions](permissions.md).

```toml
# .ai-rulez/config.toml
presets = ["claude", "cursor", "gemini", "copilot"]

[[hooks]]
event = "PreToolUse"
matcher = "Bash"
[[hooks.hooks]]
script = "scripts/guard.sh"
timeout = 10

[session_start]
generate = false   # leave out the default SessionStart hook of Claude Code
```

`generate` translates the event, the matcher, the script path and the timeout for each harness:

```json
// .claude/settings.json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "\"${CLAUDE_PROJECT_DIR}\"/'scripts/guard.sh'",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

```json
// .cursor/hooks.json
{
  "hooks": {
    "preToolUse": [
      {
        "command": "'./scripts/guard.sh'",
        "timeout": 10,
        "matcher": "^Shell$"
      }
    ]
  },
  "version": 1
}
```

Gemini CLI gets a `BeforeTool` group with the matcher `^run_shell_command$`, the command
`"$GEMINI_PROJECT_DIR"/'scripts/guard.sh'` and `"timeout": 10000` (milliseconds); Copilot gets a `preToolUse` entry in
`.github/hooks/ai-rulez.json` with `"matcher": "bash"` and `"timeoutSec": 10`.

All of this is opt-in except [`[session_start]`](session-start.md), which adds a `SessionStart` hook for
Claude Code unless you set `generate = false`.

## `[[hooks]]`

```toml
[[hooks]]
event = "PreToolUse"
matcher = "Bash"
matchers = { gemini = "run_shell_command", cursor = "Shell" }   # per-harness override
[[hooks.hooks]]
script = "scripts/guard.sh"          # or: command = "..."
timeout = 10                         # seconds
if = "Bash(git push *)"              # claude and qoder only

[[hooks]]
event = "SessionStart"
targets = ["claude", "codex"]        # optional: restrict to some harnesses
[[hooks.hooks]]
command = "ai-rulez generate --check"
status_message = "Checking generated files"
```

| Key | Type | Default | Meaning |
| --- | ---- | ------- | ------- |
| `event` | string | required | The event, by its Claude Code name; translated per harness (tables below) |
| `matcher` | string | none | Tool matcher in Claude Code's vocabulary (`Bash`, `Edit\|Write`, `mcp__github__.*`) |
| `matchers` | table | none | A matcher per harness; `matchers.<harness>` always wins over the translated `matcher` |
| `targets` | list | every harness | The harnesses the group is rendered for |
| `hooks` | list | required | Handlers, run in order |
| `hooks.command` | string | | A shell command, copied verbatim |
| `hooks.script` | string | | A committed script, relative to the project root |
| `hooks.args` | list | | Exec-form arguments (shell-quoted into the command where the harness has no exec form) |
| `hooks.timeout` | integer | harness default | Seconds; converted where the harness counts milliseconds |
| `hooks.async` | boolean | `false` | Run without waiting |
| `hooks.if` | string | | A Claude Code permission-rule condition |
| `hooks.status_message` | string | | Text shown while the hook runs, where the harness has one |

The fields are those of a [`[[plugin.hooks]]`](plugins.md) group, plus `targets` and `matchers`. Only command
handlers exist (omit `type`); `validate` and `generate` reject other types.

A matcher is copied to the harnesses that use Claude Code's tool names and rewritten token by token for the
harnesses whose vendor documents its own (see [Tool names in matchers](#tool-names-in-matchers)). A token with no
documented equivalent skips the group with a warning instead of guessing.

### Events

Events use Claude Code's names. The five harnesses with a dedicated renderer:

| Harness | File | Shape | Timeout |
| ------- | ---- | ----- | ------- |
| `claude` | `.claude/settings.json` (`hooks`) | event, matcher group, handlers | seconds |
| `codex` | `.codex/hooks.json` (`hooks`) | event, matcher group, handlers | seconds |
| `gemini` | `.gemini/settings.json` (`hooks`) | event, matcher group, handlers | milliseconds (converted) |
| `cursor` | `.cursor/hooks.json` (`version` and `hooks`) | event, flat handler entries, camelCase events | seconds |
| `copilot` | `.github/hooks/ai-rulez.json` | event, flat handler entries, `bash`, `timeoutSec` | seconds |

Supported events (`-` means the harness has no equivalent, and the group is skipped for it with a warning):

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

Gemini's `BeforeAgent` and `AfterAgent` are the nearest events to `UserPromptSubmit` and `Stop`, not exact
equivalents: `BeforeAgent` fires after a prompt is submitted and `AfterAgent` once per turn after the final response.
Review such hooks when you target Gemini.

### Handler fields

A handler field the harness lacks is never dropped when dropping it would change what the hook does:

| Field | Harnesses that have it | Elsewhere |
| ----- | ---------------------- | --------- |
| `if` | `claude`, `qoder` | the handler is skipped (running it unconditionally would widen it) |
| `async` | `claude`, `codex`, `qwen`, `qoder`, `junie`, `zcode`, the [plugin-based harnesses](#plugin-based-harnesses) | the handler is skipped |
| `args` | `claude`, `qoder`, `deepagents` | the arguments are shell-quoted into the command |
| `status_message` | where the harness has one | omitted |

A `matcher` on an event where the harness ignores matchers skips the group: Copilot and Copilot CLI honour one on
`preToolUse` and `postToolUse` only, and Cline has none.

Each skipped group or handler is reported once per run: `[[hooks]] not generated for <harness>: ...`. Presets that
have no hooks at all are named in one warning.

### Scripts

`script` is a path relative to the project root. The script is not copied: commit it, so the hook works in a fresh
clone. The generated command addresses it through the variable the harness documents for its project root:

| Harness | Command for `script = "scripts/guard.sh"` |
| ------- | ----------------------------------------- |
| `claude` | `"${CLAUDE_PROJECT_DIR}"/'scripts/guard.sh'` |
| `gemini` | `"$GEMINI_PROJECT_DIR"/'scripts/guard.sh'` |
| `codex`, `copilot` | `"$(git rev-parse --show-toplevel)"/'scripts/guard.sh'` |
| `cursor` | `'./scripts/guard.sh'` (Cursor runs project hooks from the project root) |
| the harnesses of the next section | see the script table there |

A script path may contain only letters, digits, `.`, `_`, `-` and `/`, and must stay inside the project; `validate`
rejects anything else. The generated path is single-quoted, so it cannot inject a command. `validate` reports a
script that does not exist (`AR504`) or is not executable (`AR505`), and a hook in the generated
`.claude/settings.json` that points at a missing or non-executable file (`AR501`, `AR502`).

### More settings-file harnesses

These harnesses share one renderer. The hook group is merged into the file like the five above, and
`generate --user` writes the user-level file.

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

Supported events per harness, by Claude Code name, with the native name in parentheses where it differs. An event
not listed is skipped with a warning, and so is a matcher on an event the vendor says ignores one:

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

Four mappings are the nearest event rather than one the vendor documents as equivalent: `Stop` to Vibe's
`post_agent`, Cline's `TaskComplete` and Copilot CLI's `agentStop`, and `SessionStart` to Cline's `TaskStart`.

Matchers in Claude Code names (`Bash`, `Edit|Write`) are copied as written to `claude`, `codex`, `qwen`, `codebuddy`,
`qoder`, `letta`, `grok`, `deepagents`, `junie` and `zcode`, and rewritten for the harnesses in the next section. Every
other harness (`commandcode`, `bob`, `reasonix`, `hermes`, `kimi`, ...) needs `matchers.<harness>` for a group with a
`matcher`. Two matchers are not tool names: Duo CLI's matches the session source (`startup` or `resume`), and Vibe's
`match` is a glob (or a regular expression after `re:`). Poolside and Vibe require a matcher and get `*` when the group
sets none.

### Tool names in matchers

A matcher is rewritten token by token over its top-level alternation (`Edit|Write`, `^Bash$`, `mcp__github__.*`). A
Claude Code name outside the harness's documented vocabulary, a regular expression more complex than a name, or an
alternation where the harness matches a glob or an exact name skips the whole group with a warning that names the
token; a translation is never partial. Where the harness tests the matcher as an unanchored regular expression, each
name is anchored (`^Execute$`), because a Claude Code matcher matches the whole name.

| Harness | Claude Code tool to native name | MCP tools | Vendor page |
| --- | --- | --- | --- |
| `cursor` | Bash `Shell`; Read `Read`; Edit, MultiEdit, Write `Write` (Cursor files edits under Write); Grep `Grep`; Task, Agent `Task` | not rewritten (`MCP:<tool>` carries no server) | cursor.com/docs/hooks |
| `gemini` | Bash `run_shell_command`; Read `read_file`, `read_many_files`; Edit, MultiEdit `replace`; Write `write_file`; Grep `grep_search`; Glob `glob`; LS `list_directory`; WebFetch `web_fetch`; WebSearch `google_web_search`; TodoWrite `write_todos` | `mcp_<server>_<tool>` | geminicli.com/docs/reference/tools/ |
| `copilot`, `copilot-cli` | Bash `bash`; Read `view`; Edit, MultiEdit `edit`; Write `create`; Grep `grep`; Glob `glob`; WebFetch `web_fetch`; Task, Agent `task` | not documented | docs.github.com/en/copilot/reference/hooks-configuration |
| `factory` | Bash `Execute`; Read `Read`; Edit `Edit`; Write `Create`; Grep `Grep`; Glob `Glob`; LS `LS`; WebFetch `FetchUrl`; WebSearch `WebSearch`; Task, Agent `Task` | `mcp__<server>__<tool>` | docs.factory.com/reference/hooks-reference |
| `devin` | Bash `exec`; Read `read`; Edit `edit`; Write `write`; Grep `grep`; Glob `glob`; WebFetch `webfetch` | `mcp__<server>__<tool>` | docs.devin.ai/cli/extensibility/hooks/lifecycle-hooks |
| `kiro` | Bash `shell` (one tool, no alternation; Kiro's other categories span several Claude Code tools) | not rewritten | kiro.dev/docs/hooks/types/ |
| `goose` | Bash `shell`; Write `write`; Edit, MultiEdit `edit` | `<server>__<tool>` | goose-docs.ai/docs/guides/context-engineering/hooks/ |
| `crush` | Bash `bash`; Read `view`; Edit `edit`; Write `write`; MultiEdit `multiedit`; Grep `grep`; Glob `glob`; LS `ls`; Task, Agent `agent` | `mcp_<server>_<tool>` | github.com/charmbracelet/crush docs/hooks/README.md |
| `cortex` | Bash `bash`; Read `read`; Edit `edit`; Write `write`; Grep `grep`; Glob `glob`; NotebookEdit `notebook_edit_cell` | `mcp__<server>__<tool>` | docs.snowflake.com/en/user-guide/cortex-code/extensibility |
| `poolside` | Bash `shell`; Read `read`; Edit, MultiEdit `edit`; Write `write`; WebFetch `web_fetch`; WebSearch `web_search` | not documented | docs.poolside.ai/hooks |
| `vibe` | Bash `bash`; Grep `grep` (a glob: one tool, no alternation) | not rewritten | docs.mistral.ai/vibe/code/cli/hooks |
| `augment` | Bash `launch-process`; Read `view`; Edit, MultiEdit `str-replace-editor`; Write `save-file`; WebFetch `web-fetch`; WebSearch `web-search` | not rewritten (`mcp:` prefix, `<tool>_<server>`) | docs.augmentcode.com/cli/hooks |
| `antigravity` | Bash `run_command`; Read `view_file`; Edit `replace_file_content`; MultiEdit `multi_replace_file_content`; Write `write_to_file`; Grep `grep_search`; Glob `find_by_name`; LS `list_dir`; WebFetch `read_url_content`; WebSearch `search_web`; Task, Agent `invoke_subagent` | not documented | antigravity.google/docs/hooks |

Command Code, Bob and Reasonix document too few tool names to map, so set `matchers.<harness>` for them. For the
plugin-based `amp`, only `Bash`, `Read`, `Grep` and `Task` map to documented Amp names; the others (`edit_file`,
`create_file`, `glob`, ...) are not on Amp's documentation pages, so check them with `amp tools list` and set
`matchers.amp` if one differs. Cursor's
`beforeShellExecution` is not used for `PreToolUse` with `Bash`: its matcher is tested against the command text, not
the tool name.

The script variable for these harnesses:

| Harness | Command for `script = "scripts/guard.sh"` |
| ------- | ----------------------------------------- |
| `qwen` | `"$QWEN_PROJECT_DIR"/'scripts/guard.sh'` |
| `codebuddy` | `"$CODEBUDDY_PROJECT_DIR"/'scripts/guard.sh'` |
| `qoder` | `"$QODER_PROJECT_DIR"/'scripts/guard.sh'` |
| `commandcode` | `"$COMMANDCODE_PROJECT_DIR"/'scripts/guard.sh'` |
| `factory` | `"$FACTORY_PROJECT_DIR"/'scripts/guard.sh'` |
| `devin` | `"$DEVIN_PROJECT_DIR"/'scripts/guard.sh'` |
| `cortex` | `"$CORTEX_PROJECT_DIR"/'scripts/guard.sh'` |
| `gitlab-duo` | `"$DUO_PROJECT_DIR"/'scripts/guard.sh'` |
| `grok` | `"$GROK_WORKSPACE_ROOT"/'scripts/guard.sh'` |
| `deepagents` | `"${CLAUDE_PROJECT_DIR}"/'scripts/guard.sh'` |
| `crush` | `"$CRUSH_PROJECT_DIR"/'scripts/guard.sh'` |
| `letta`, `antigravity`, `bob`, `kiro`, `vibe` | `'./scripts/guard.sh'` (the harness runs project hooks from the project directory) |
| `cline` | `"$root"/scripts/guard.sh`, where the wrapper computes `$root` from its own location |
| `augment`, `goose`, `poolside`, `reasonix` | not generated: the harness documents no way to address a project file, so use `command` |

With `generate --user`, every `script` becomes its absolute path in the user config directory.

Some harnesses need a step from you before a generated project hook runs: Duo CLI needs `--enable-project-hooks` (or
`GITLAB_ENABLE_PROJECT_HOOKS=true`), Grok needs `/hooks-trust`, and Deep Agents asks you to trust the file. Devin also
loads the hooks of `.claude/settings.json`, so a hook generated for both `claude` and `devin` runs twice. `generate`
prints each of these once per run.

`codewhale` gets no hooks: its project hooks load only after you approve a digest of the file's exact bytes, and its
events do not map to Claude Code's.

#### Cline

Cline runs the executable named after an event from `.clinerules/hooks/` (and `~/Documents/Cline/Hooks/`), so
`generate` writes one POSIX shell script per event (`PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `PreCompact`,
`TaskStart`, `TaskComplete`). The script runs the configured commands in order with the hook input on stdin; the first
failing command stops the rest, and the last command's output is the hook's output. Cline has no matcher, timeout or
`async`, so a group or handler that sets one is skipped with a warning. Windows `.ps1` hooks are not generated.

A script is treated as generated only when its second line starts with `# Generated by ai-rulez`; a script of yours
with the same name is never overwritten.

### Plugin-based harnesses

OpenCode, Kilo, MiMo Code, Pi and Amp load hooks as code from a plugin or extension directory. For them `[[hooks]]`
render into one JavaScript or TypeScript module that ai-rulez owns whole:

| Harness | Project file | User file (`--user`) | Plugin API |
| ------- | ------------ | -------------------- | ---------- |
| `opencode` | `.opencode/plugins/ai-rulez-hooks.js` | `~/.config/opencode/plugins/ai-rulez-hooks.js` | default export with the v2 `setup(ctx)` and the v1 `server(ctx)`, so it loads on either |
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

An event outside the column is skipped with a warning. OpenCode also reports `session.created` and `session.idle` for
subagent sessions, so those hooks run for subagents too.

How the module runs a command:

- **Input.** The command runs in a shell from the project directory, with a Claude Code style JSON document on
  stdin: `session_id`, `cwd`, `hook_event_name` and the event's fields (`tool_name`, `tool_input`, `tool_response`,
  `prompt`, `source`, `trigger`, `reason`). `tool_name` is the Claude Code name where one is mapped (`bash` becomes
  `Bash`) and `harness_tool_name` the harness's own. `CLAUDE_PROJECT_DIR`, `AI_RULEZ_PROJECT_DIR` and
  `AI_RULEZ_HOOK_EVENT` are set.
- **Matcher.** A regular expression that must match a whole tool name, tested against the harness's name and the
  Claude Code names mapped to it, so `Edit|Write` selects OpenCode's `edit`, `write` and `apply_patch`. An unmapped
  name (an MCP tool) matches only by its own name; write it in `matchers.<harness>`. On `SessionStart` the matcher is
  tested against the source, on Pi's compaction events against `manual` or `auto`. A matcher that Go and JavaScript
  would read differently (inline flags, lookarounds, backreferences, unbalanced groups) skips the group with a
  warning.
- **Blocking.** Exit code 2 from a `PreToolUse` hook blocks the call and its stderr becomes the reason, as does a JSON
  decision of `block` or `deny` on stdout (Pi also blocks a prompt on `UserPromptSubmit`). Any other non-zero exit, a
  timeout (`timeout`, default 600 seconds) or a spawn failure is logged and never blocks.
- **Scripts.** A `script` runs as `./<script>` from the project directory; `args` are shell-quoted into the command.

Amp requires a `tool.call` handler to return a result; a non-blocking hook returns `{ action: "allow" }`, the only
result that neither rejects nor rewrites the call.

The module is owned only while its first line is the `// Generated by ai-rulez` banner; a plugin of yours with the same
file name is left alone with a warning. With `gitignore = true` the module is gitignored; `clean`, or dropping the last
hook, removes it.

### Failure and environment

- **Hooks fail open.** Only a deliberate block (exit code 2, or a JSON `block`/`deny` decision) stops an event. Any
  other non-zero exit, a timeout, a missing script or a spawn failure is logged and the event proceeds. A guard that
  must not be bypassed has to exit 2 on every error path (`set -e` exits 1). A deny that matters also belongs in
  [`[permissions]`](permissions.md) or a sandbox.
- **Full environment.** A hook command inherits the harness's environment, credentials included, and runs with your
  privileges. Review `[[hooks]]` like code.
- **Duplicates.** A harness that merges several hook sources runs them all, so a hook generated for two overlapping
  harnesses (Devin and Claude Code) can run twice.

### Trust and review

Harnesses do not run project hooks blindly. Codex requires non-managed hooks to be reviewed and trusted (`/hooks`)
and loads project hooks only for a trusted `.codex/` layer; Claude Code asks you to trust the folder before project
`allow` rules and most `env` values apply. Generating a hook does not bypass any of that. `generate` prints new or
changed hook commands the first time it writes them; `--yes` or `AI_RULEZ_ACK_COMMANDS=1` silences the notice.

## `[guard]`

```toml
[guard]
generated = true
# version = "5.10.0"         # npx version to run; default is the generating binary's version
# command = ["ai-rulez"]     # replaces `npx -y ai-rulez@<version>`; excludes version
```

With `generated = true`, `generate` adds a `PreToolUse` hook that runs `ai-rulez guard` before every file-editing tool
call and blocks the calls that target a generated file. The agent is told to edit the source under `.ai-rulez/`
instead. An agent that ignores the "do not edit" banner otherwise loses its edit on the next `generate`.

| Key | Type | Default | Meaning |
| --- | ---- | ------- | ------- |
| `generated` | boolean | `false` | Add the guard hook |
| `version` | string | the generating binary's version | The version `npx -y ai-rulez@<version> guard` runs |
| `command` | list | `["npx", "-y", "ai-rulez@<version>"]` | The launcher, without the trailing `guard`; excludes `version` |

Only harnesses whose documented `PreToolUse` hook blocks a call on exit code 2 get the guard; `generate` logs which
others were skipped:

| Harness | Event | Matcher it renders |
| ------- | ----- | ------------------ |
| `claude` | `PreToolUse` | `Edit\|Write\|MultiEdit` |
| `codex` | `PreToolUse` | `apply_patch\|Edit\|Write` |
| `gemini` | `BeforeTool` | `^replace$\|^write_file$` |
| `cursor` | `preToolUse` | `^Write$` |
| `factory` | `PreToolUse` | `Edit\|Create\|ApplyPatch` |
| `copilot` | `preToolUse` | `edit\|create` |
| `copilot-cli` | `preToolUse` | `edit\|create` |

The first `npx` run downloads the package and can exceed the 10-second hook timeout; the hook then fails open and the
edit goes through. With a local install, set `command = ["ai-rulez"]`. `generate --user` never adds the guard.

How `ai-rulez guard` decides:

- It reads the hook JSON on stdin (`tool_input.file_path`, `path`, `target_file`, or the `*** Update File:` lines of a
  Codex `apply_patch`), resolves `..` and symlinks, and locates the project from the payload's `cwd`.
- It blocks a path only when it is a wholly generated output listed in `.ai-rulez/.generated-manifest.json` or the
  machine-local `.generated-manifest.local.json`. Documents ai-rulez only merges keys into (`.claude/settings.json`,
  `.vscode/settings.json`, `.mcp.json`, ...) and read-only tools are never blocked.
- It reads the two manifests and nothing else: no config load, no network.
- It fails open on its own errors (a payload that does not parse, no manifest, a path outside the project). A blocked
  call exits 2 with the reason on stderr. An edit payload over 8 MiB, or one naming more than 1,000 files, is blocked
  with a request for smaller edits.

The guard stops file-editing tools only. A write through a shell (`sed -i`, `tee`, a `>` redirect) is not blocked;
[`ai-rulez generate --check`](cli.md#detecting-drift) still catches the hand edit afterwards.

## `[permissions]`

```toml
[permissions]
allow = ["Bash(git status)", "Bash(git diff *)"]
ask   = ["Bash(git push *)"]
deny  = ["Read(./.env)", "Bash(rm -rf *)"]
```

Rules in Claude Code syntax, translated into the native permission file of every harness that has one. The rule
syntax, the per-harness files and what each translation can express are in [Permissions](permissions.md).

## `[claude.settings.managed]`

Currently supports Claude Code only.

```toml
[claude.settings.managed]
env = { EXAMPLE_FLAG = "1" }
skill_overrides = { init = "off", legacy-deploy = "name-only" }
```

| Key | Writes | Values |
| --- | ------ | ------ |
| `env` | `env.<NAME>` of `.claude/settings.json` | strings |
| `skill_overrides` | `skillOverrides.<skill>` | `on`, `name-only`, `user-invocable-only`, `off` |

Each entry is owned on its own; other entries of the same objects survive. Declaring an entry is the opt-in. An
entry the file already holds with the configured value stays yours on `clean`; one with a different value is replaced
on `generate` and removed on `clean`.

A [role](roles.md)'s `skill_mode` is written through the same keys: `generate --role <name>` owns
`skillOverrides.<skill>` for the skills the role sets, the role's value wins over `skill_overrides`, and switching
roles removes the previous role's entries.

## Files and ownership

| File | Written when | On `clean` |
| ---- | ------------ | ---------- |
| `.claude/settings.json` | the project generates for `claude` (the default [`[session_start]`](session-start.md) hook), or `[[hooks]]`, `[permissions]`, managed keys or plugin keys are declared | the entries ai-rulez wrote are removed; the file goes when nothing else is left |
| `.claude/settings.json`, `.codex/config.toml`, `opencode.json` | `[[enabled_plugins]]` are declared (see [Enable a plugin from another marketplace](plugins.md#enable-a-plugin-from-another-marketplace)) | same |
| `.codex/hooks.json`, `.cursor/hooks.json`, `.gemini/settings.json` | `[[hooks]]` apply to the harness | same |
| `.factory/hooks.json`, `.agents/hooks.json`, `.devin/hooks.v1.json`, `.gitlab/duo/hooks.json`, `.grok/hooks/ai-rulez.json`, `.kiro/hooks/ai-rulez.json`, `.deepagents/hooks.json`, `.agents/plugins/ai-rulez/hooks/hooks.json`, `.vibe/hooks.toml` | `[[hooks]]` apply to the harness | same |
| `.qwen/settings.json`, `.augment/settings.json`, `.codebuddy/settings.json`, `.qoder/settings.json`, `.commandcode/settings.json`, `.letta/settings.json`, `.bob/settings.json`, `.cortex/settings.json`, `.reasonix/settings.json`, `crush.json`, `.poolside/settings.yaml` | `[[hooks]]` or `[permissions]` apply to the harness, or MCP servers for `qwen`, `augment`, `crush` and `poolside` | same; MCP servers, hooks and permissions of one file are merged together |
| `~/.junie/config.json`, `~/.zcode/cli/config.json`, `~/.hermes/config.yaml`, `~/.kimi-code/config.toml` | `generate --user` with `[[hooks]]` | same; ZCode's `hooks.enabled` is added only when absent |
| `.opencode/plugins/ai-rulez-hooks.js`, `.kilo/plugins/ai-rulez-hooks.js`, `.mimocode/plugins/ai-rulez-hooks.js`, `.pi/extensions/ai-rulez-hooks.ts`, `.amp/plugins/ai-rulez-hooks.ts` | `[[hooks]]` apply to the harness | deleted; hand-written plugins in the directory stay |
| `.clinerules/hooks/<Event>` | `[[hooks]]` apply to `cline` | deleted |
| `.github/hooks/ai-rulez.json` | `[[hooks]]` apply to `copilot` or `copilot-cli` | deleted; Copilot loads every `*.json` in the directory, so put your own hook files beside it |

The files that hold only permission rules (`.codex/rules/ai-rulez.rules`, `.cursor/cli.json`, `.vscode/settings.json`,
...) are listed in [Permissions](permissions.md#harnesses).

A document with comments or trailing commas (JSONC) is edited in place and keeps its comments; a document that does not
parse fails with a hint naming the path. Project-wide settings are not written for `[[scopes]]` subdirectories.

A settings document that ai-rulez creates is gitignored with the other generated files when `gitignore = true`, so
its hooks reach teammates only when the file was already tracked, or with `gitignore = false`. Hook commands are quoted
for POSIX shells; the Copilot `powershell` field is not written.

### Shared settings documents

`.mcp.json`, `.claude/settings.json`, `.gemini/settings.json`, `.amp/settings.json`, `.agents/mcp_config.json`,
`.pi/mcp.json`, `opencode.json` and similar files are shared documents: ai-rulez owns specific entries and you own the
rest. `generate` replaces only its entries and keeps every other member byte for byte, including indentation and, in
JSONC, TOML and YAML files, comments.

- **MCP servers are owned one by one.** A server you added under a name that is not in your configuration survives
  `generate` and `clean`. A server removed from the configuration is removed on the next `generate`, as long as it
  still holds the value ai-rulez wrote; an entry you edited is yours and stays, with a warning. A hand-written server
  with the name of a configured one is replaced by the configured value.
- **Hooks, permissions and managed keys** are owned element by element: hand-written groups, rules and entries in the
  same arrays survive.
- **Plugin keys.** With `[claude.settings] manage = true`, ai-rulez owns `extraKnownMarketplaces.<marketplace>` and the
  listed `enabledPlugins` entries, each on its own.
- **Written only when needed.** A settings document is written only when ai-rulez has something to put in it (MCP
  servers, a resolved Amp effort, managed keys, hooks). `.gemini/settings.json` (`context.fileName`) and
  `opencode.json` (`instructions`) also carry the entry that loads machine-local content.
- **Removal.** `clean`, and `generate` after a preset or server is removed, take back the entries ai-rulez merged in
  while they hold the value it wrote, and keep the rest of the document. A key or object left empty is dropped, and
  a document left with nothing is deleted. The record of what was merged is kept as
  digests, never values; see [Settings documents shared with you](local-overrides.md#settings-documents-shared-with-you).
- **Git ignore.** A document that still holds keys you own is your file: it is not added to the managed `.gitignore`
  block and never deleted as stale. A document holding only ai-rulez's keys is a generated file and is ignored with
  `gitignore = true`.

### What ai-rulez may change

ai-rulez takes an entry back, or deletes a file, only on proof that it wrote it:

1. an in-file `Content-Hash` that matches the file's body, or
2. a claim or digest in the machine-local manifest (`.ai-rulez/.generated-manifest.local.json`, gitignored), or
3. the current configuration rendering the same value.

The shared manifest (`.ai-rulez/.generated-manifest.json`) lists generated paths for teammates and the guard, but
anyone with push access can edit it, so it is never accepted as proof. What follows from that:

- An entry you edited after `generate` wrote it is yours and stays.
- A settings document that existed before ai-rulez first wrote to it is yours: `clean` never deletes it, and values
  you already had are not claimed.
- `generate` followed by `clean` restores the original bytes of a merged document.
- A fresh clone has no machine-local manifest. A hook you drop from the configuration stays in a committed
  `.claude/settings.json` (and the other merged documents) on such a clone, and `generate --check` reports no drift
  for it. Remove the stale group by hand.
- `generate` rewrites a generated file whose body no longer matches its `Content-Hash`, so a hand edit is lost. `clean`
  keeps such a file and warns; `clean --include-edited` removes it.
- The machine-local manifest is believed only when git does not track it and only you can write to it; otherwise
  `generate` warns (run `git rm --cached .ai-rulez/.generated-manifest.local.json`).
- `generate` and `clean` never remove a file whose directory resolves, through a symlink, outside the project.

## Vendor documentation

The hook formats follow each vendor's documentation:

| Harness | Page |
| ------- | ---- |
| `claude` | `code.claude.com/docs/en/hooks`, `/settings`, `/skills` (`skillOverrides`) |
| `codex` | `learn.chatgpt.com/docs/hooks` |
| `cursor` | `cursor.com/docs/hooks` |
| `gemini` | `geminicli.com/docs/hooks/` |
| `copilot`, `copilot-cli` | `docs.github.com/en/copilot/reference/hooks-configuration` |
| `opencode` | `opencode.ai/docs/plugins/` |
| `kilo` | `kilo.ai/docs/automate/extending/plugins` |
| `pi` | `packages/coding-agent/docs/extensions.md` in `earendil-works/pi` |
| `amp` | `ampcode.com/docs/plugin-api` |
| `factory` | `docs.factory.com/reference/hooks-reference` |
| `antigravity` | `antigravity.google/docs/hooks` |
| `qwen` | `qwenlm.github.io/qwen-code-docs/en/users/features/hooks/` |
| `augment` | `docs.augmentcode.com/cli/hooks` |
| `codebuddy` | `codebuddy.ai/docs/cli/hooks` |
| `qoder` | `docs.qoder.com/cli/hooks` |
| `commandcode` | `commandcode.ai/docs/hooks` |
| `letta` | `docs.letta.com/letta-code/hooks` |
| `gitlab-duo` | `docs.gitlab.com/user/gitlab_duo_cli/customize/` (hooks are an experiment) |
| `devin` | `docs.devin.ai/cli/extensibility/hooks/overview` |
| `grok` | `docs.x.ai/build/features/hooks` |
| `bob` | `bob.ibm.com/docs/ide/configuration/lifecycle-hooks` |
| `cortex` | `docs.snowflake.com/en/user-guide/cortex-code/extensibility` |
| `goose` | `goose-docs.ai/docs/guides/context-engineering/hooks/` |
| `deepagents` | `docs.langchain.com/oss/deepagents/code/hooks` |
| `junie` | `junie.jetbrains.com/docs/junie-cli-hooks.html` |
| `zcode` | `zcode.z.ai/en/docs/hooks` |
| `crush` | `docs/hooks/README.md` in `github.com/charmbracelet/crush` |
| `poolside` | `docs.poolside.ai/hooks` |
| `hermes` | `hermes-agent.nousresearch.com/docs/user-guide/features/hooks` |
| `kiro` | `kiro.dev/docs/hooks/` |
| `vibe` | `docs.mistral.ai/vibe/code/cli/hooks` |
| `kimi` | `moonshotai.github.io/kimi-code/en/customization/hooks.html` |
