# Supported harnesses

ai-rulez has 52 built-in presets, one per AI coding harness. A preset writes the harness's native layout: its file
names, frontmatter, directories and settings documents. List the presets you use in `config.toml`; for a harness that
is not listed, write a [provider-backed custom preset](configuration.md#provider-backed-presets-full-parity).

```toml
# .ai-rulez/config.toml
presets = ["claude", "cursor", "codex", "gemini"]
```

```bash
ai-rulez generate
```

With one path-scoped rule, one always-on rule, a skill, an agent, a command and an MCP server, those four presets
write:

```text
AGENTS.md                         # every rule for codex and gemini (the path-scoped one inlined); imported by CLAUDE.md
CLAUDE.md                         # @AGENTS.md
.mcp.json                         # MCP servers (Claude Code reads the root file)
.claude/settings.json             # the default [session_start] hook
.claude/rules/api.md              # path-scoped rule
.claude/skills/deploy/SKILL.md
.claude/skills/ship/SKILL.md      # the command, as a user-invocable skill
.claude/agents/reviewer.md
.cursor/rules/api.mdc
.cursor/commands/ship.md
.cursor/agents/reviewer.md
.cursor/mcp.json
.agents/skills/deploy/SKILL.md    # shared by codex, cursor and gemini
.agents/skills/ship/SKILL.md      # the command, as a codex skill
.agents/skills/ship/agents/openai.yaml
.codex/agents/reviewer.toml
.codex/config.toml                # MCP servers
.gemini/agents/reviewer.md
.gemini/settings.json             # MCP servers
```

Codex and Gemini CLI have no rules folder, so the path-scoped rule is also inlined into `AGENTS.md`, which Claude
Code and Cursor read too; `generate` warns about it and suggests `[rules] scoped_fallback = "skill"`.

`ai-rulez init` lists every built-in preset in a comment of the new `config.toml`. Three more preset names are
not harnesses: `mcp` (only the root `.mcp.json`), [`okf`](okf.md) and [`llms-txt`](llms-txt.md).

## Feature matrix

`yes` means the preset writes that output at project level, `-` that it does not (the reason is under
[Skipped features](#skipped-features)).

- **Rules**: a native rules folder. `-` means rules are inlined into the root instructions file (`letta` has none,
  so it gets no rules).
- **Commands**: slash commands. Where the harness has no command folder, a command is written as a skill (`claude`,
  `codex`, `antigravity`, `devin`, `warp`) or as a prompt or workflow file (`copilot`, `pi`, `cline`, `kiro`).
- **Hooks** and **Perms**: `yes` renders into a project file; `user` renders only with
  [`generate --user`](user-scope.md), because the harness reads them from the user configuration only. See
  [Hooks and settings](settings.md) and [Permissions](permissions.md).
- **Checks**: [code-review guidelines](checks.md).
- `copilot-cli` permissions cover only `WebFetch` URL rules, and `gitlab-duo` hooks only the `SessionStart` event.
- **User**: supported by `generate --user`; see [User-level configuration](user-scope.md#where-things-go).

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

## What each preset writes

The paths below are the project-level outputs with the default `agents_md = true`: always-on rules and context go
into one shared `AGENTS.md` and skills into one shared `.agents/skills/`, which every preset that reads them uses
instead of its own copy (see [AGENTS.md and .agents/skills](agents-md.md)). `<id>` is the item's name. Every project
also gets the root `.mcp.json`, whatever its presets: it carries the ai-rulez MCP server
([`[mcp] self_server`](configuration.md#mcp), on by default) and any servers you declare.

### Instructions, rules and skills

A rules folder holds the path-scoped rules; with `agents_md = false` it also holds the always-on rules, and the
presets marked * write their own root file and skills folder instead (next table).

| Preset | Instructions | Rules | Skills |
| ------ | ------------ | ----- | ------ |
| `aiassistant` | none | `.aiassistant/rules/<id>.md` (every rule) | `.agents/skills/` |
| `amp` | `AGENTS.md` | inlined | `.agents/skills/` |
| `antigravity` * | `AGENTS.md` | `.agents/rules/<id>.md` | `.agents/skills/` |
| `augment` * | `AGENTS.md` | `.augment/rules/<id>.md` | `.agents/skills/` |
| `baz` | `AGENTS.md`, `<dir>/AGENTS.md` ([Baz](baz.md)) | nested `AGENTS.md` | `.agents/skills/` |
| `bob` | `AGENTS.md` | `.bob/rules/<id>.md` | `.bob/skills/` |
| `claude` | `CLAUDE.md` (imports `@AGENTS.md`) | `.claude/rules/<id>.md` | `.claude/skills/` |
| `cline` * | `AGENTS.md` | `.clinerules/<id>.md` | `.agents/skills/` |
| `codebuddy` | `CODEBUDDY.md` | `.codebuddy/rules/<id>.md` (every rule) | `.codebuddy/skills/` |
| `codebuff` | `AGENTS.md` | inlined | `.agents/skills/` |
| `codewhale` | `AGENTS.md` | `.codewhale/rules/<id>.md` | `.codewhale/skills/` |
| `codex` | `AGENTS.md` | inlined | `.agents/skills/` |
| `commandcode` | `AGENTS.md` | inlined | `.commandcode/skills/` |
| `copilot`, `copilot-cli` * | `AGENTS.md` | `.github/instructions/<id>.instructions.md` | `.agents/skills/` |
| `cortex` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `crush` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `cursor` | `AGENTS.md` | `.cursor/rules/<id>.mdc` | `.agents/skills/` |
| `deepagents` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `devin` * | `AGENTS.md` | `.devin/rules/<id>.md` | `.agents/skills/` |
| `dsh` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `factory` | `AGENTS.md` | inlined | `.factory/skills/` |
| `gemini` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `gitlab-duo` | `.gitlab/duo/chat-rules.md` | inlined | none |
| `goose` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `grok` | `AGENTS.md` | `.grok/rules/<id>.md` | `.grok/skills/` |
| `hermes` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `junie` * | `AGENTS.md` | `.junie/rules/<id>.md` | `.agents/skills/` |
| `kilo` | `AGENTS.md` | `.kilo/rules/<id>.md` | `.kilo/skills/` |
| `kimi` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `kiro` * | `AGENTS.md` | `.kiro/steering/<id>.md` | `.agents/skills/` |
| `letta` | none | not written | `.agents/skills/` |
| `mimocode` | `AGENTS.md` | inlined | `.mimocode/skills/` |
| `muse`, `pi` | `AGENTS.md` | inlined | `.agents/skills/` |
| `omp` * | `AGENTS.md` | `.omp/rules/<id>.md` | `.agents/skills/` |
| `openclaw` | `AGENTS.md` | inlined | none |
| `opencode` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `poolside` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `qoder` | `AGENTS.md` | `.qoder/rules/<id>.md` | `.qoder/skills/` |
| `qwen` * | `AGENTS.md` | `.qwen/rules/<id>.md` | `.qwen/skills/` |
| `reasonix` | `REASONIX.md` | inlined | `.reasonix/skills/` |
| `replit` | `replit.md` | inlined | `.agents/skills/` |
| `rovodev` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `takt` | none | `.takt/facets/policies/<id>.md` (every rule) | `.takt/facets/knowledge/<id>.md` |
| `trae` | none | `.trae/rules/<id>.md` (every rule) | `.trae/skills/` |
| `vibe` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `warp` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `xum` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `zcode` | `AGENTS.md` | inlined | `.zcode/skills/` |
| `zed` * | `AGENTS.md` | inlined | `.agents/skills/` |
| `zoocode` * | `AGENTS.md` | inlined | `.agents/skills/` |

With `agents_md = false`, the presets marked * write:

| Preset | Instructions | Skills |
| ------ | ------------ | ------ |
| `antigravity` | `GEMINI.md` | `.agents/skills/` |
| `augment` | `AGENTS.md` | `.augment/skills/` |
| `cline` | none (every rule in `.clinerules/`) | `.cline/skills/` |
| `copilot`, `copilot-cli` | `.github/copilot-instructions.md` | `.github/skills/` |
| `cortex` | `AGENTS.md` | `.cortex/skills/` |
| `crush` | `CRUSH.md` | `.crush/skills/` |
| `deepagents` | `AGENTS.md` | `.deepagents/skills/` |
| `devin` | none (every rule in `.devin/rules/`) | `.devin/skills/` |
| `dsh` | `AGENTS.md` | `.dsh/skills/` |
| `gemini` | `GEMINI.md` | `.agents/skills/` |
| `goose` | `.goosehints` | `.agents/skills/` |
| `hermes` | `.hermes.md` | none |
| `junie` | `AGENTS.md` | `.junie/skills/` |
| `kimi` | `AGENTS.md` | `.kimi-code/skills/` |
| `kiro` | `AGENTS.md` | `.kiro/skills/` |
| `omp` | `AGENTS.md` | `.omp/skills/` |
| `opencode` | `AGENTS.md` | `.opencode/skills/` |
| `poolside` | `AGENTS.md` | `.poolside/skills/` |
| `qwen` | `QWEN.md` | `.qwen/skills/` |
| `rovodev` | `AGENTS.md` | `.rovodev/skills/` |
| `vibe` | `AGENTS.md` | `.vibe/skills/` |
| `warp` | `AGENTS.md` | `.warp/skills/` |
| `xum` | `AGENTS.md` | `.xum/skills/` |
| `zed` | `.rules` | `.agents/skills/` |
| `zoocode` | `AGENTS.md` | `.roo/skills/` |

### Agents, commands, MCP and settings

Settings files are written only when the project declares [hooks](settings.md) or [permissions](permissions.md),
except `.claude/settings.json`, which also holds the [`[session_start]`](session-start.md) hook that a `claude` project
gets by default;
`(user)` marks a file that only `generate --user` writes.

| Preset | Agents | Commands | MCP | Hooks and permissions |
| ------ | ------ | -------- | --- | --------------------- |
| `aiassistant` | - | - | `.ai/mcp/mcp.json` | - |
| `amp` | - | - | `.amp/settings.json` | `.amp/plugins/ai-rulez-hooks.ts` |
| `antigravity` | `.agents/agents/<id>.md` | `.agents/skills/<id>/SKILL.md` | `.agents/mcp_config.json` | `.agents/hooks.json` |
| `augment` | `.augment/agents/<id>.md` | `.augment/commands/<id>.md` | `.augment/settings.json` | `.augment/settings.json` |
| `baz` | `.claude/agents/<id>.md` | - | - | - |
| `bob` | - | `.bob/commands/<id>.md` | `.bob/mcp.json` | `.bob/settings.json` |
| `claude` | `.claude/agents/<id>.md` | `.claude/skills/<id>/SKILL.md` | `.mcp.json` | `.claude/settings.json` |
| `cline` | `.cline/agents/<id>.yaml` | `.clinerules/workflows/<id>.md` | - | `.clinerules/hooks/<Event>` |
| `codebuddy` | `.codebuddy/agents/<id>.md` | `.codebuddy/commands/<id>.md` | `.mcp.json` | `.codebuddy/settings.json` |
| `codebuff` | - | - | `.agents/mcp.json` | - |
| `codewhale` | - | `.codewhale/commands/<id>.md` | `.codewhale/mcp.json` | - |
| `codex` | `.codex/agents/<id>.toml` | `.agents/skills/<id>/SKILL.md` | `.codex/config.toml` | `.codex/hooks.json`, `.codex/rules/ai-rulez.rules` |
| `commandcode` | `.commandcode/agents/<id>.md` | `.commandcode/commands/<id>.md` | `.mcp.json` | `.commandcode/settings.json` |
| `copilot` | `.github/agents/<id>.agent.md` | `.github/prompts/<id>.prompt.md` | `.vscode/mcp.json` | `.github/hooks/ai-rulez.json`, `.vscode/settings.json` |
| `copilot-cli` | `.github/agents/<id>.agent.md` | - | `.github/mcp.json` | `.github/hooks/ai-rulez.json`, `.github/copilot/settings.json` |
| `cortex` | `.cortex/agents/<id>.md` | - | - | `.cortex/settings.json` |
| `crush` | - | - | `crush.json` | `crush.json` |
| `cursor` | `.cursor/agents/<id>.md` | `.cursor/commands/<id>.md` | `.cursor/mcp.json` | `.cursor/hooks.json`, `.cursor/cli.json` |
| `deepagents` | `.deepagents/agents/<id>/AGENTS.md` | - | `.deepagents/.mcp.json` | `.deepagents/hooks.json` |
| `devin` | `.devin/agents/<id>.md` | `.agents/skills/<id>/SKILL.md` | `.devin/mcp_config.json` | `.devin/hooks.v1.json`, `.devin/config.json` |
| `dsh` | - | - | - | - |
| `factory` | `.factory/droids/<id>.md` | `.factory/commands/<id>.md` | `.factory/mcp.json` | `.factory/hooks.json` |
| `gemini` | `.gemini/agents/<id>.md` | - | `.gemini/settings.json` | `.gemini/settings.json` |
| `gitlab-duo` | - | `.agents/commands/<id>.md` | `.gitlab/duo/mcp.json` | `.gitlab/duo/hooks.json` |
| `goose` | `.goose/agents/<id>.md` | - | `.agents/plugins/ai-rulez/.mcp.json` | `.agents/plugins/ai-rulez/hooks/hooks.json` |
| `grok` | `.grok/agents/<id>.md` | `.grok/commands/<id>.md` | `.grok/config.toml` | `.grok/hooks/ai-rulez.json`, `.grok/config.toml` |
| `hermes` | - | - | - | `~/.hermes/config.yaml` (user) |
| `junie` | `.junie/agents/<id>.md` | `.junie/commands/<id>.md` | `.junie/mcp/mcp.json` | `~/.junie/config.json` (user) |
| `kilo` | `.kilo/agents/<id>.md` | `.kilo/commands/<id>.md` | `kilo.jsonc` | `.kilo/plugins/ai-rulez-hooks.js`, `kilo.jsonc` |
| `kimi` | `.kimi-code/agents/<id>.md` | - | `.kimi-code/mcp.json` | `~/.kimi-code/config.toml` (user) |
| `kiro` | `.kiro/agents/<id>.md` | `.kiro/prompts/<id>.md` | `.kiro/settings/mcp.json` | `.kiro/hooks/ai-rulez.json` |
| `letta` | `.letta/agents/<id>.md` | `.commands/<id>.md` | - | `.letta/settings.json` |
| `mimocode` | `.mimocode/agents/<id>.md` | `.mimocode/commands/<id>.md` | `.mimocode/mimocode.jsonc` | `.mimocode/plugins/ai-rulez-hooks.js`, `.mimocode/mimocode.jsonc` |
| `muse` | - | - | - | - |
| `omp` | `.omp/agents/<id>.md` | `.omp/commands/<id>.md` | `.omp/mcp.json` | `.omp/config.yml` |
| `openclaw` | - | - | - | - |
| `opencode` | `.opencode/agents/<id>.md` | `.opencode/commands/<id>.md` | `opencode.json` | `.opencode/plugins/ai-rulez-hooks.js`, `opencode.json` |
| `pi` | `.pi/agents/<id>.md` | `.pi/prompts/<id>.md` | `.pi/mcp.json` | `.pi/extensions/ai-rulez-hooks.ts` |
| `poolside` | - | - | `.poolside/settings.yaml` | `.poolside/settings.yaml` |
| `qoder` | `.qoder/agents/<id>.md` | `.qoder/commands/<id>.md` | `.mcp.json` | `.qoder/settings.json` |
| `qwen` | `.qwen/agents/<id>.md` | `.qwen/commands/<id>.md` | `.qwen/settings.json` | `.qwen/settings.json` |
| `reasonix` | `.reasonix/skills/<id>/SKILL.md` | `.reasonix/commands/<id>.md` | `.mcp.json` | `.reasonix/settings.json` |
| `replit` | - | - | - | - |
| `rovodev` | `.rovodev/subagents/<id>.md` | - | - | - |
| `takt` | `.takt/facets/personas/<id>.md` | `.takt/facets/instructions/<id>.md` | - | - |
| `trae` | - | - | `.trae/mcp.json` | - |
| `vibe` | - | - | `.vibe/config.toml` | `.vibe/hooks.toml`, `.vibe/config.toml` |
| `warp` | - | `.agents/skills/<id>/SKILL.md` | `.warp/.mcp.json` | - |
| `xum` | `.xum/agents/<id>.md` | - | `.xum/mcp.jsonc` | - |
| `zcode` | `.zcode/agents/<id>.md` | `.zcode/commands/<id>.md` | `.zcode/config.json` | `~/.zcode/cli/config.json` (user) |
| `zed` | - | - | `.zed/settings.json` | `~/.config/zed/settings.json` (user) |
| `zoocode` | - | `.roo/commands/<id>.md` | `.roo/mcp.json` | `.vscode/settings.json` |

[Checks](checks.md) are written by eight presets: `amp` (`.agents/checks/<id>.md`), `augment`
(`.augment/code_review_guidelines.yaml`), `cursor` (`.cursor/BUGBOT.md`), `factory`
(`.factory/skills/review-guidelines/SKILL.md`), `gitlab-duo` (`.gitlab/duo/mr-review-instructions.yaml`), `kilo`
(`REVIEW.md`), `qwen` (`.qwen/review-rules.md`) and `rovodev` (`.rovodev/.review-agent.md`).

User-level paths are listed in [User-level configuration](user-scope.md#where-things-go).

## Cross-cutting behavior

- **Shared outputs.** Presets that write the same path (`AGENTS.md`, `.agents/skills/`, the root `.mcp.json`) must
  render identical bytes. `generate` fails and names the presets when they diverge. See
  [AGENTS.md and .agents/skills](agents-md.md).
- **Merged documents.** Settings files that also hold your own keys (`.claude/settings.json`, `opencode.json`,
  `.codex/config.toml`, `.qwen/settings.json`, ...) are merged, not replaced: ai-rulez owns only the keys, array
  elements and map members it writes, and comments in JSONC, TOML and YAML survive. See
  [Settings document merge behavior](configuration.md#settings-document-merge-behavior).
- **External plugins.** `[[enabled_plugins]]` enables a plugin from another marketplace for `claude`
  (`enabledPlugins`, `extraKnownMarketplaces`), `codex` (`[plugins."<id>"]`) and `opencode` (the `plugin` array).
  The other presets have no equivalent and skip it. See
  [Enable a plugin from another marketplace](plugins.md#enable-a-plugin-from-another-marketplace).
- **MCP environment references.** Where the harness expands `${VAR}`-style references itself, the reference is
  written instead of the secret. See [Environment references](configuration.md#environment-references).
- **Built-in names.** A skill, command or agent named like a harness built-in replaces it, is renamed, or is
  unreachable, depending on the harness. `validate` checks every output against the harnesses that load it,
  including those that only read a shared directory such as `.agents/skills/`: AR9U0 for a built-in name, AR9U1
  for a name the harness refuses to load. See
  [Built-in names of harnesses](strict-validation.md#built-in-names-of-harnesses).
- **Tool and model names.** Frontmatter tool names are translated to each harness's own names, and bare Claude model
  aliases (`sonnet`, `opus`, `haiku`) are dropped where the harness has no such model.

## Skipped features

A `-` in the matrix is a deliberate skip: ai-rulez does not approximate a format the harness does not document.

### Instructions, rules, skills and agents

| Preset | Skipped | Reason |
| ------ | ------- | ------ |
| `aiassistant` | root file | AI Assistant has no root instructions file; every rule and context item becomes a `.aiassistant/rules/` file |
| `amp` | agents | Amp has no agent file of its own and does not read the `.agents/agents` format |
| `baz` | rules folder, commands, MCP | Baz reads only instruction files from the default branch; it ignores `.claude/commands` and does not read `.claude/rules`. See [Baz](baz.md) |
| `bob` | agents | Bob's modes live in an aggregated `custom_modes.yaml`, not in one file per agent |
| `codebuff` | rules, agents, commands | Codebuff documents only `AGENTS.md`, skills and MCP |
| `codewhale` | agents | Subagents are TOML profiles whose strict schema the preset cannot produce |
| `crush` | rules folder, agents, commands | Crush has no rules folder, agent or command format |
| `cline` | MCP | Cline has no project MCP file |
| `cortex` | rules folder, MCP | Cortex reads only `AGENTS.md`; MCP servers live in `~/.snowflake/cortex/mcp.json` |
| `dsh` | MCP | Servers live in the home-level `cordis.patch.yml` |
| `gemini` | rules folder, commands | Rules stay in the root file; project commands are not generated (see the [plugin runtime](plugins.md) for `commands/<name>.toml`) |
| `gitlab-duo` | skills, rules folder | GitLab documents skills only inside plugins; Duo CLI reads one rules file, `.gitlab/duo/chat-rules.md` |
| `goose` | rules folder, commands | goose has no rules folder, and recipes are not slash commands |
| `hermes` | skills | Skills come only from the shared `.agents/skills` that `agents_md = true` writes |
| `letta` | rules, MCP | Letta documents no instructions, rules or MCP file a project can use |
| `muse` | MCP | MCP servers live only in the user `settings.json` |
| `openclaw` | everything but the root file | Only `AGENTS.md` is written |
| `poolside` | agents | Subagents are entries of the settings file, which the preset does not generate |
| `replit` | rules, agents, commands, MCP | Replit reads `replit.md` and `.agents/skills` only |
| `rovodev` | commands, MCP | Saved prompts need a `prompts.yml` manifest; `.rovodev/mcp.json` is inert until `config.yml` points at it |
| `takt` | MCP | Takt keeps MCP definitions inside workflow YAML |
| `trae` | root file | Trae has no root instructions file |
| `vibe` | agents | A Vibe agent is a TOML profile plus a separate prompt file |
| `xum` | commands | Xum documents no slash-command file |
| `zoocode` | agents | Custom modes are one aggregated `.roomodes` file |
| `zed` | agents, commands, rules folder | Zed has no rules folder, subagent or command format; rules go to the root file |
| `amp`, `codex`, `commandcode`, `deepagents`, `dsh`, `factory`, `hermes`, `kimi`, `mimocode`, `muse`, `opencode`, `pi`, `poolside`, `reasonix`, `rovodev`, `vibe`, `warp`, `xum`, `zcode`, `zoocode` | rules folder | The harness has no rules folder (or one it does not auto-load); rules are inlined into the root file |
| `aiassistant`, `dsh`, `gitlab-duo`, `hermes`, `muse`, `trae`, `warp` | agents | The harness documents no agent file |
| `aiassistant`, `amp`, `copilot-cli`, `cortex`, `deepagents`, `dsh`, `hermes`, `kimi`, `muse`, `poolside`, `trae`, `vibe` | commands | The harness documents no command file |
| `hermes` | MCP | The harness documents no project MCP file |

### Hooks

Sixteen presets generate no `[[hooks]]`: `aiassistant`, `baz`, `codebuff`, `codewhale`, `dsh`, `muse`, `omp`,
`openclaw`, `replit`, `rovodev`, `takt`, `trae`, `warp`, `xum`, `zed` and `zoocode`. `generate` names them in one
warning. `junie`, `zcode`, `hermes` and `kimi` read hooks from the user configuration only, so they get hooks from
`generate --user`. Events, matchers and timeouts per harness are in
[Hooks and settings](settings.md#more-settings-file-harnesses).

### Permissions

The harnesses without a documented, committable permission file that can hold the rules get nothing from
`[permissions]`, and `generate` names them in a warning. The reasons are in [Permissions](permissions.md#not-translated).
`zed`, `hermes` and `kimi` read permissions from the user configuration only.

### User scope

`aiassistant`, `baz`, `codebuff`, `replit` and `xum` have no documented user-level location for anything
`generate --user` writes, so they are reported and skipped. User-level MCP servers are never generated.
