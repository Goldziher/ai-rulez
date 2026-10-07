# Supported harnesses

ai-rulez ships 52 built-in presets, one per AI coding harness. The table below lists them; `ai-rulez init` also writes the list into a comment in the new `config.toml`. Each preset
writes the harness's own layout: file names, frontmatter, directories and settings documents. For a harness that
is not listed, use a [provider-backed custom preset](configuration.md#provider-backed-presets-full-parity).

The shared `mcp` preset (the root `.mcp.json`) is not a harness and is not in the table.

## Feature matrix

`yes` means the preset writes that output at project level. `-` means it does not; the reason is under
[Skipped features](#skipped-features). Terms:

- **Rules**: a native rules folder. `-` means rules are inlined into the root instructions file.
- **Commands**: slash commands. A command is written as a skill where the harness has no command folder
  (`codex`, `antigravity`, `devin`, `warp`) and as a prompt or workflow file for `copilot`, `pi`, `cline` and
  `kiro` (`.kiro/prompts`). `claude` still reads `.claude/commands`, but ai-rulez writes commands there as
  user-invocable skills, because Claude Code merges the two.
- **Hooks** and **Perms** (permissions): `yes` renders into a project file, `user` renders only with
  [`generate --user`](user-scope.md) because the vendor reads them from the user configuration only.
  See [Hooks, permissions and settings keys](settings.md) and [Permissions](permissions.md).
- **Checks**: [code-review guidelines](checks.md).
- **User**: supported by `generate --user`; see [User-level configuration](user-scope.md) for the paths.

| Preset | Harness | Rules | Skills | Agents | Commands | MCP | Hooks | Perms | Checks | User |
| ------ | ------- | ----- | ------ | ------ | -------- | --- | ----- | ----- | ------ | ---- |
| `aiassistant` | [JetBrains AI Assistant](https://www.jetbrains.com/help/ai-assistant/) | yes | yes | - | - | yes | - | - | - | - |
| `amp` | [Sourcegraph Amp](https://ampcode.com) | - | yes | - | - | yes | yes | - | yes | yes |
| `antigravity` | [Google Antigravity](https://antigravity.google/docs) | yes | yes | yes | yes | yes | yes | - | - | yes |
| `augment` | [Augment Code](https://docs.augmentcode.com) | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| `baz` | [Baz](https://baz.ai/docs/agents/skills-and-instructions) | - | yes | yes | - | - | - | - | - | - |
| `bob` | [IBM Bob](https://bob.ibm.com/docs) | yes | yes | - | yes | yes | yes | - | - | yes |
| `claude` | [Claude Code](https://code.claude.com/docs) | yes | yes | yes | yes | yes | yes | yes | - | yes |
| `cline` | [Cline](https://docs.cline.bot) | yes | yes | yes | yes | - | yes | - | - | yes |
| `codebuddy` | [CodeBuddy Code](https://www.codebuddy.ai/docs/cli) | yes | yes | yes | yes | yes | yes | yes | - | yes |
| `codebuff` | [Codebuff](https://www.codebuff.com/docs) | - | yes | - | - | yes | - | - | - | - |
| `codewhale` | [Codewhale](https://github.com/codewhale-hq/Codewhale) | yes | yes | - | yes | yes | - | - | - | yes |
| `codex` | [OpenAI Codex](https://developers.openai.com/codex) | - | yes | yes | yes | yes | yes | yes | - | yes |
| `commandcode` | [Command Code](https://commandcode.ai/docs) | - | yes | yes | yes | yes | yes | yes | - | yes |
| `copilot` | [GitHub Copilot](https://docs.github.com/en/copilot) | yes | yes | yes | yes | yes | yes | yes | - | yes |
| `copilot-cli` | [GitHub Copilot CLI](https://docs.github.com/en/copilot/how-tos/copilot-cli) | yes | yes | yes | - | yes | yes | yes | - | yes |
| `cortex` | [Snowflake Cortex Code](https://docs.snowflake.com/en/user-guide/cortex-code/extensibility) | - | yes | yes | - | - | yes | - | - | yes |
| `crush` | [Crush](https://github.com/charmbracelet/crush) | - | yes | - | - | yes | yes | - | - | yes |
| `cursor` | [Cursor](https://cursor.com/docs) | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| `deepagents` | [Deep Agents Code](https://docs.langchain.com/oss/deepagents/code) | - | yes | yes | - | yes | yes | - | - | yes |
| `devin` | [Devin CLI](https://docs.devin.ai/cli) | yes | yes | yes | yes | yes | yes | yes | - | yes |
| `dsh` | [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) | - | yes | - | - | - | - | - | - | yes |
| `factory` | [Factory Droid](https://docs.factory.ai) | - | yes | yes | yes | yes | yes | - | yes | yes |
| `gemini` | [Gemini CLI](https://geminicli.com/docs/) | - | yes | yes | - | yes | yes | yes | - | yes |
| `gitlab-duo` | [GitLab Duo CLI](https://docs.gitlab.com/user/gitlab_duo_cli/customize/) | - | - | - | yes | yes | yes | - | yes | yes |
| `goose` | [goose](https://goose-docs.ai) | - | yes | yes | - | yes | yes | - | - | yes |
| `grok` | [Grok Build CLI](https://docs.x.ai/build/overview) | yes | yes | yes | yes | yes | yes | yes | - | yes |
| `hermes` | [Hermes Agent](https://hermes-agent.nousresearch.com/docs) | - | `agents_md` | - | - | - | user | user | - | yes |
| `junie` | [JetBrains Junie](https://junie.jetbrains.com) | yes | yes | yes | yes | yes | user | - | - | yes |
| `kilo` | [Kilo Code](https://kilo.ai/docs) | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| `kimi` | [Kimi Code](https://moonshotai.github.io/kimi-code/) | - | yes | yes | - | yes | user | user | - | yes |
| `kiro` | [Kiro](https://kiro.dev/docs) | yes | yes | yes | yes | yes | yes | - | - | yes |
| `letta` | [Letta Code](https://docs.letta.com) | - | yes | yes | yes | - | yes | yes | - | yes |
| `mimocode` | [MiMo Code](https://mimo.xiaomi.com/mimocode) | - | yes | yes | yes | yes | yes | yes | - | yes |
| `muse` | [Meta Muse Code](https://dev.meta.ai/docs/muse-code) | - | yes | - | - | - | - | - | - | yes |
| `omp` | [oh-my-pi](https://github.com/can1357/oh-my-pi) | yes | yes | yes | yes | yes | - | yes | - | yes |
| `openclaw` | [OpenClaw](https://docs.openclaw.ai) | - | - | - | - | - | - | - | - | yes |
| `opencode` | [OpenCode](https://opencode.ai/docs) | - | yes | yes | yes | yes | yes | yes | - | yes |
| `pi` | [pi](https://pi.dev/docs/latest) | - | yes | yes | yes | yes | yes | - | - | yes |
| `poolside` | [Poolside Pool](https://docs.poolside.ai) | - | yes | - | - | yes | yes | yes | - | yes |
| `qoder` | [Qoder](https://docs.qoder.com/cli/quickstart) | yes | yes | yes | yes | yes | yes | yes | - | yes |
| `qwen` | [Qwen Code](https://qwenlm.github.io/qwen-code-docs/en/) | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| `reasonix` | [Reasonix](https://github.com/esengine/DeepSeek-Reasonix) | - | yes | yes | yes | yes | yes | - | - | yes |
| `replit` | [Replit Agent](https://docs.replit.com/features/agent/overview) | - | yes | - | - | - | - | - | - | - |
| `rovodev` | [Atlassian Rovo Dev](https://www.atlassian.com/software/rovo-dev) | - | yes | yes | - | - | - | - | yes | yes |
| `takt` | [Takt](https://github.com/nrslib/takt) | yes | yes | yes | yes | - | - | - | - | yes |
| `trae` | [Trae](https://docs.trae.ai/ide/rules) | yes | yes | - | - | yes | - | - | - | yes |
| `vibe` | [Mistral Vibe](https://docs.mistral.ai/vibe) | - | yes | - | - | yes | yes | yes | - | yes |
| `warp` | [Warp](https://docs.warp.dev) | - | yes | - | yes | yes | - | - | - | yes |
| `xum` | [Xum](https://xum.coder.com) | - | yes | yes | - | yes | - | - | - | - |
| `zcode` | [ZCode](https://zcode.z.ai/en/docs) | - | yes | yes | yes | yes | user | - | - | yes |
| `zed` | [Zed](https://zed.dev/docs/ai) | - | yes | - | - | yes | - | user | - | yes |
| `zoocode` | [Zoo Code](https://docs.zoocode.dev) | - | yes | - | yes | yes | - | yes | - | yes |

Where each output lands (file names, merged documents, user-level paths) is in the preset's provider spec under
`internal/generator/providers/builtin/<preset>.toml` and, for the Go presets (`antigravity`, `baz`, `cline`,
`codex`, `copilot`, `cursor`, `devin`, `gemini`, `opencode`, `xum`), in `internal/generator/presets/`. The
user-level table is in [User-level configuration](user-scope.md#where-things-go).

## Cross-cutting behavior

- **Shared outputs.** Presets that write the same path (`AGENTS.md`, `.agents/skills/`, the root `.mcp.json`)
  must render identical bytes. `generate` fails and names the presets when they diverge, except for an
  `AGENTS.md` that omits rules because its harness reads them from a rules folder; the complete file is kept
  with a warning. See [AGENTS.md and .agents/skills](agents-md.md).
- **Merged documents.** Settings files that hold your own keys (`.claude/settings.json`, `opencode.json`,
  `.codex/config.toml`, `.qwen/settings.json`, ...) are merged, not replaced. ai-rulez owns the keys, array
  elements and map members it writes, and comments in JSONC, TOML and YAML survive. See
  [Settings document merge behavior](configuration.md#settings-document-merge-behavior).
- **Native MCP environment references.** Where the harness expands `${VAR}`-style references itself, the
  reference is written instead of the secret. See
  [Environment references](configuration.md#mcp_servers).
- **Native tool and model names.** Frontmatter is translated to each harness's own tool names, and bare Claude
  model aliases (`sonnet`, `opus`, `haiku`) are dropped where the harness has no such model.

## Skipped features

A missing feature is a deliberate skip, with the reason taken from the provider spec or the settings and
permissions docs. Nothing is approximated.

### Instructions, rules, skills and agents

| Preset | Skipped | Reason |
| ------ | ------- | ------ |
| `aiassistant` | root file | AI Assistant has no root instructions file, so every rule and context item becomes a `.aiassistant/rules/*.md` file |
| `amp` | agents | `.agents/agents` is an Antigravity format Amp does not read; agents are only listed in `AGENTS.md` |
| `baz` | rules folder, commands, MCP | Baz has no repository config file and reads only instruction files from the default branch; it ignores `.claude/commands` and does not read `.claude/rules`. See [Baz](baz.md) |
| `bob` | agents | Bob has no file-based subagents; its modes live in a Roo-style `custom_modes.yaml` aggregate |
| `codebuff` | rules, agents, commands | Codebuff documents only a knowledge file (`AGENTS.md`), skills and MCP |
| `codewhale` | agents | Subagents are TOML profiles with a deny-unknown-fields schema the provider DSL cannot express |
| `crush` | rules folder, agents, commands | Crush has inline guidance (`CRUSH.md`), skills and MCP only |
| `cline` | MCP | Cline has no project MCP file |
| `cortex` | rules folder, MCP | Cortex reads only `AGENTS.md`; project MCP is not documented (servers live in `~/.snowflake/cortex/mcp.json`) |
| `dsh` | MCP | Servers live in the home-level `cordis.patch.yml`, which a project preset cannot express |
| `gemini` | rules folder, commands | Rules stay in the root file (`AGENTS.md`, or `GEMINI.md` with `agents_md = false`); project commands are not generated (see the [plugin runtime](plugins.md) for `commands/<name>.toml`) |
| `gitlab-duo` | skills, rules folder | GitLab documents skills only inside plugins; Duo CLI reads one rules file, `.gitlab/duo/chat-rules.md` |
| `goose` | rules folder, commands | goose has no rules folder; recipes are not slash commands |
| `hermes` | skills | With `agents_md = false` only project context is written (`.hermes.md`); skills come from the shared `.agents/skills`, which `agents_md` (the default) writes |
| `kimi`, `deepagents`, `factory`, `zcode`, `mimocode`, `zoocode`, `warp` | rules folder | No rules folder (or one the harness does not auto-load): rules are inlined into the root file. `kilo` keeps `.kilo/rules` and registers it in `kilo.jsonc` |
| `letta` | rules, MCP | No rules or MCP file the tool documents |
| `muse` | MCP | MCP servers live only in the user `settings.json` |
| `openclaw` | everything but the root file | Only the shared root `AGENTS.md` is written; no rules folder or skills layout can be targeted |
| `poolside` | agents | Subagents are entries of the settings file, which the preset does not generate |
| `replit` | rules, agents, commands, MCP | Replit has `replit.md` and `.agents/skills` only |
| `rovodev` | commands, MCP | Saved prompts need a `prompts.yml` manifest; `.rovodev/mcp.json` is inert until `config.yml` points at it |
| `takt` | MCP | Takt keeps MCP definitions inside workflow YAML. Rules, skills, agents and commands map to facets under `.takt/facets/` |
| `trae` | root file | Trae has no root instructions file; the preset writes rule files, skills and MCP |
| `vibe` | agents | A Vibe agent is a TOML profile whose prompt is a second file, which one output cannot produce |
| `xum` | commands | The tool documents no slash-command file |
| `amp`, `codex`, `commandcode`, `dsh`, `hermes`, `muse`, `opencode`, `pi`, `poolside`, `reasonix`, `rovodev`, `vibe`, `xum` | rules folder | The tool documents no rules folder: rules are inlined into the root instructions file (`AGENTS.md`, `.hermes.md` for `hermes`, `REASONIX.md` for `reasonix`) |
| `aiassistant`, `dsh`, `gitlab-duo`, `hermes`, `muse`, `trae`, `warp` | agents | The tool documents no such file |
| `zoocode` | agents | Custom modes are an aggregated `.roomodes` YAML file, which the provider DSL cannot produce |
| `aiassistant`, `amp`, `copilot-cli`, `cortex`, `deepagents`, `dsh`, `hermes`, `kimi`, `muse`, `poolside`, `trae`, `vibe` | commands | The tool documents no such file |
| `hermes` | MCP | The tool documents no project MCP file |
| `zed` | agents, commands, rules folder | Zed has no rules folder, subagent or command file format; rules go to the root `.rules` file |

### Hooks

Sixteen presets generate no `[[hooks]]`: `aiassistant`, `baz`, `codebuff`, `codewhale`, `dsh`, `muse`, `omp`,
`openclaw`, `replit`, `rovodev`, `takt`, `trae`, `warp`, `xum`, `zed` and `zoocode`. A preset without hook
support is named in one `generate` warning; ai-rulez emits no hook it could not read from the vendor's
documentation.

`user` in the matrix marks harnesses whose vendor reads hooks (or permissions) only from the user configuration:
`junie` ("ignores project hooks"), `zcode` (ignores project hooks), `hermes`, `kimi` (user `config.toml`
only) and, for permissions, `zed`. They render with `generate --user`. Per-harness event, matcher and timeout
details are in [Hooks, permissions and settings keys](settings.md#more-settings-file-harnesses).

### Permissions

These harnesses have no documented, committable permission file that can express `[permissions]`, so they get
nothing from it and `generate` names them in a warning (from [Permissions](permissions.md#not-translated)):

| Preset | Reason |
| ------ | ------ |
| `amp` | Current Amp docs removed `amp.permissions`; only whole-tool `amp.tools.disable` remains |
| `pi` | No permission mechanism; `defaultTools` only enables or disables whole tools |
| `cline` | Command permissions are documented only as the `CLINE_COMMAND_PERMISSIONS` environment variable |
| `kiro` | Conflicting documented shapes in `.kiro/agents/*.json`; the workspace permissions file lives outside the repository |
| `factory` | The project settings path is documented as `settings.local.json`, and `commandDenylist` means "ask", not "deny" |
| `crush` | `crush.json` is deprecated for `crushrc`; its schema has `allowed_tools` and no deny |
| `rovodev` | The bash rules location differs between Atlassian's pages and their match order is undocumented |
| `takt` | Only a coarse permission mode (`readonly`, `edit`, `full`), no rules |
| `antigravity` | Only the user-level CLI file is documented; the project path is not |
| `goose`, `junie`, `deepagents`, `warp` | User-level files with whole-tool, allow-only, exact-command or regex semantics that cannot hold the rules without widening them |

Every other preset without a `yes` or `user` in the Perms column has no permission surface in its provider spec.

### Checks

Only eight harnesses read code-review guidelines from a repository file: `amp`, `augment`, `cursor`,
`factory`, `gitlab-duo`, `kilo`, `qwen` and `rovodev`. Not generated: Augment area grouping and per-area globs,
GitLab `fileFilters`, Takt quality gates, Hermes pre-verify specs, and JetBrains AI Assistant (its self-review
path is a per-user IDE setting). See [Checks](checks.md#where-they-are-written).

### User scope

`aiassistant`, `baz`, `codebuff`, `replit` and `xum` have no documented user-level location for anything
`generate --user` writes and are reported and skipped. `codebuff` has only an MCP file, which user scope does not
generate. User-level MCP servers are never generated.
