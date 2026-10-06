# User-level configuration

`ai-rulez generate` writes inside the project. `ai-rulez generate --user` does the same for a person: it
renders a user config into the per-user directories each harness reads (`~/.claude`, `~/.agents/skills`,
`~/.codex`, `~/.gemini`, `~/.config/opencode`, `~/.copilot`, `~/.pi/agent`, and so on for every harness
that documents one; see [Where things go](#where-things-go)), so personal or role-based
instructions and skills follow you across every repository, including ones that do not use ai-rulez.

It is opt-in: nothing is written to the home directory unless you pass `--user`.

## The user config

The user config has the same schema as a project config and lives in `~/.config/ai-rulez/` (or
`$XDG_CONFIG_HOME/ai-rulez`, or any directory with `--config <dir>`):

```text
~/.config/ai-rulez/
  config.toml
  rules/        skills/        agents/        domains/
```

```toml
version = "4.0"
name = "me"
presets = ["claude", "codex", "gemini"]
default = "backend"          # the role of this machine; see below

[profiles]
backend = ["backend"]
frontend = ["frontend"]
```

`presets`, `profiles`, `default`, `builtins`, `includes`, `installed_skills`, `[[hooks]]`, `[permissions]`
and `[claude.settings.managed]` work as in a project. The project-only features do nothing at user level:
MCP servers, `[[scopes]]`, `[plugin]`/`[marketplace]`, `agents_md`, the `.local` overlay and gitignore
management. A preset with no documented user-level location (see the table) is reported with the reason and
skipped.

`generate --user` ignores the project in the working directory entirely. It reads it only to warn when a
skill of the same name exists at both levels.

### Roles

The simplest role is a profile. Keep the shared content at the top level and one domain per role, then choose the
role per machine with `default` in the user config or per run with `--profile`:

```bash
ai-rulez generate --user --profile frontend
```

Switching the role removes the files of the old one on the next run (they are in the manifest).

For finer control, declare `[[roles]]` in the user config (domains, per-kind `include` / `exclude` selectors,
inheritance and a per-skill `skill_mode` that becomes `skillOverrides` in `~/.claude/settings.json`) and select one
with `--role`, which is mutually exclusive with `--profile`:

```bash
ai-rulez generate --user --role backend-engineer --yes
```

ai-rulez does not decide who holds a role: an external tool picks the name. See [Roles](roles.md), including
[Integrating an identity tool or UI](roles.md#integrating-an-identity-tool-or-ui).

## Commands

```bash
ai-rulez generate --user --dry-run     # list every path that would be written, merged, skipped or removed
ai-rulez generate --user               # same list, then asks before writing (interactive terminal)
ai-rulez generate --user --yes         # no prompt (required in a non-interactive shell)
ai-rulez clean --user                  # remove exactly what generate --user wrote
```

`--config <dir>` selects another user config. `--user` cannot be combined with `--recursive`, `--plugin` or
a config-file argument. The full write list is printed before the first write, whatever the flags.

## Safety

- **Only documented locations.** An output is written only where the preset's declared user-level layout
  (the table below) places it. Anything else a preset renders (project-only rules folders, MCP files,
  `.claude/plugins.json`) is dropped and counted; `--debug` names each one.
- **Hand-authored files are never replaced.** A file that exists and was not written by ai-rulez (not in the
  manifest, no generated banner, not byte-identical to the output) is skipped and listed as `skip:`. A skill
  directory holding one is skipped whole. `~/.claude/CLAUDE.md` you wrote yourself stays yours; ai-rulez
  writes nothing at that path.
- **Shared settings documents are merged, not rewritten.** `~/.claude/settings.json`, `~/.codex/hooks.json`,
  `~/.cursor/hooks.json` and `~/.gemini/settings.json` get only the keys ai-rulez owns (`hooks`,
  `permissions.*`, `env`, `skillOverrides` from [Hooks and permissions](settings.md)); every other key
  survives `generate` and `clean`. The generated-file [`[guard]`](settings.md#guard) is never added: it protects a project, not the home directory.
- **No symlink escapes.** Every destination, resolved through symlinks, must stay inside the home directory (or the relocated tool home, see below).
  A `~/.claude` symlinked into `~/dotfiles` is followed; one pointing out of the home directory stops the
  run before anything is written. A symlinked file is skipped, except for the settings documents above.
- **Clean removes only what was recorded.** The manifest is `~/.config/ai-rulez/.generated-manifest.json`
  (plus `.generated-manifest.local.json` for the keys merged into shared documents). `clean --user`,
  and `generate --user` after you drop content, remove a file only if it is listed there and, for files
  that carry a generated header, still looks generated. The manifest also records the directories `generate --user` created below your home directory
  (`~/.cursor`, `~/.claude/skills/<id>`, ...); `clean --user` removes those once empty. A directory that
  existed before ai-rulez ran is never removed, and neither is one that still holds a file of yours. A
  manifest written by an older version records no directories, so the first `generate --user` after upgrading
  starts the record. A relocated tool home is never removed, and the user config is never touched beyond its
  two manifest files. Keep `~/.config/ai-rulez` in your dotfiles repository if you
  like, but add the two manifest files to its ignore list.
- **`--dry-run` writes nothing**, not even the manifest.

## Where things go

There is no separate table in the code. Each preset declares its user-level layout once, next to its
project layout: a provider spec in its `[global]` block (plus `global_path` on a settings sidecar), a Go
preset through `GlobalOutputProvider`. `generate --user` renders the preset as for a project and maps each
output onto that layout. An output the preset has no user-level location for (MCP files, plugin sidecars,
a kind of content the tool has no home folder for) is dropped and counted in the plan. The table below is
generated from those declarations and a test fails when it drifts (`UPDATE_DOCS=1 go test
./internal/generator/userscope` rewrites it).

Kinds: `instructions` is the root instructions file, `rules` a rules folder, `skills`, `agents` and
`commands` the matching folders, `settings` a shared config document of the tool. At user level only
[hooks, permissions and managed settings](settings.md) are merged into a `settings` document, never MCP
servers, and nothing is written to it when the config has none. Claude Code renders commands as skills, so
they share the `skills` row.

<!-- user-scope-table:start -->
| Harness | Output | Project path | User-level path |
| ------- | ------ | ------------ | --------------- |
| `amp` | instructions | `AGENTS.md` | `~/.config/amp/AGENTS.md` |
| `amp` | skills | `.agents/skills/` | `~/.config/agents/skills/` |
| `amp` | settings | `.amp/plugins/ai-rulez-hooks.ts` | `~/.config/amp/plugins/ai-rulez-hooks.ts` |
| `amp` | settings | `.amp/settings.json` | `~/.config/amp/settings.json` |
| `antigravity` | instructions | `GEMINI.md` | `~/.gemini/GEMINI.md` |
| `antigravity` | rules | `.agents/rules/` | `~/.gemini/config/rules/` |
| `antigravity` | skills | `.agents/skills/` | `~/.gemini/config/skills/` |
| `antigravity` | agents | `.agents/agents/` | `~/.gemini/config/agents/` |
| `antigravity` | settings | `.agents/hooks.json` | `~/.gemini/config/hooks.json` |
| `antigravity` | settings | `.agents/mcp_config.json` | `~/.gemini/config/mcp_config.json` |
| `augment` | rules | `.augment/rules/` | `~/.augment/rules/` |
| `augment` | skills | `.augment/skills/` | `~/.augment/skills/` |
| `augment` | agents | `.augment/agents/` | `~/.augment/agents/` |
| `augment` | commands | `.augment/commands/` | `~/.augment/commands/` |
| `augment` | settings | `.augment/settings.json` | `~/.augment/settings.json` |
| `bob` | instructions | `AGENTS.md` | `~/.bob/AGENTS.md` |
| `bob` | rules | `.bob/rules/` | `~/.bob/rules/` |
| `bob` | skills | `.bob/skills/` | `~/.bob/skills/` |
| `bob` | commands | `.bob/commands/` | `~/.bob/commands/` |
| `bob` | settings | `.bob/mcp.json` | `~/.bob/settings/mcp.json` |
| `bob` | settings | `.bob/settings.json` | `~/.bob/settings/settings.json` |
| `claude` | instructions | `CLAUDE.md` | `~/.claude/CLAUDE.md` |
| `claude` | rules | `.claude/rules/` | `~/.claude/rules/` |
| `claude` | skills | `.claude/skills/` | `~/.claude/skills/` |
| `claude` | agents | `.claude/agents/` | `~/.claude/agents/` |
| `claude` | settings | `.claude/settings.json` | `~/.claude/settings.json` |
| `cline` | rules | `.clinerules/` | `~/Documents/Cline/Rules/` |
| `cline` | skills | `.cline/skills/` | `~/.cline/skills/` |
| `cline` | agents | `.cline/agents/` | `~/.cline/agents/` |
| `cline` | commands | `.clinerules/workflows/` | `~/Documents/Cline/Workflows/` |
| `cline` | settings | `.clinerules/hooks/PostToolUse` | `~/Documents/Cline/Hooks/PostToolUse` |
| `cline` | settings | `.clinerules/hooks/PreCompact` | `~/Documents/Cline/Hooks/PreCompact` |
| `cline` | settings | `.clinerules/hooks/PreToolUse` | `~/Documents/Cline/Hooks/PreToolUse` |
| `cline` | settings | `.clinerules/hooks/TaskComplete` | `~/Documents/Cline/Hooks/TaskComplete` |
| `cline` | settings | `.clinerules/hooks/TaskStart` | `~/Documents/Cline/Hooks/TaskStart` |
| `cline` | settings | `.clinerules/hooks/UserPromptSubmit` | `~/Documents/Cline/Hooks/UserPromptSubmit` |
| `codebuddy` | instructions | `CODEBUDDY.md` | `~/.codebuddy/CODEBUDDY.md` |
| `codebuddy` | rules | `.codebuddy/rules/` | `~/.codebuddy/rules/` |
| `codebuddy` | skills | `.codebuddy/skills/` | `~/.codebuddy/skills/` |
| `codebuddy` | agents | `.codebuddy/agents/` | `~/.codebuddy/agents/` |
| `codebuddy` | commands | `.codebuddy/commands/` | `~/.codebuddy/commands/` |
| `codebuddy` | settings | `.codebuddy/settings.json` | `~/.codebuddy/settings.json` |
| `codewhale` | instructions | `AGENTS.md` | `~/.codewhale/AGENTS.md` |
| `codewhale` | skills | `.codewhale/skills/` | `~/.codewhale/skills/` |
| `codewhale` | commands | `.codewhale/commands/` | `~/.codewhale/commands/` |
| `codewhale` | settings | `.codewhale/mcp.json` | `~/.codewhale/mcp.json` |
| `codex` | instructions | `AGENTS.md` | `~/.codex/AGENTS.md` |
| `codex` | skills | `.agents/skills/` | `~/.agents/skills/` |
| `codex` | agents | `.codex/agents/` | `~/.codex/agents/` |
| `codex` | settings | `.codex/config.toml` | `~/.codex/config.toml` |
| `codex` | settings | `.codex/hooks.json` | `~/.codex/hooks.json` |
| `codex` | settings | `.codex/rules/ai-rulez.rules` | `~/.codex/rules/ai-rulez.rules` |
| `commandcode` | instructions | `AGENTS.md` | `~/.commandcode/AGENTS.md` |
| `commandcode` | skills | `.commandcode/skills/` | `~/.commandcode/skills/` |
| `commandcode` | agents | `.commandcode/agents/` | `~/.commandcode/agents/` |
| `commandcode` | commands | `.commandcode/commands/` | `~/.commandcode/commands/` |
| `commandcode` | settings | `.commandcode/settings.json` | `~/.commandcode/settings.json` |
| `commandcode` | settings | `.mcp.json` | `~/.commandcode/mcp.json` |
| `copilot` | instructions | `.github/copilot-instructions.md` | `~/.copilot/copilot-instructions.md` |
| `copilot` | rules | `.github/instructions/` | `~/.copilot/instructions/` |
| `copilot` | skills | `.github/skills/` | `~/.copilot/skills/` |
| `copilot` | agents | `.github/agents/` | `~/.copilot/agents/` |
| `copilot` | settings | `.github/hooks/ai-rulez.json` | `~/.copilot/hooks/ai-rulez.json` |
| `copilot-cli` | instructions | `.github/copilot-instructions.md` | `~/.copilot/copilot-instructions.md` |
| `copilot-cli` | rules | `.github/instructions/` | `~/.copilot/instructions/` |
| `copilot-cli` | skills | `.github/skills/` | `~/.copilot/skills/` |
| `copilot-cli` | agents | `.github/agents/` | `~/.copilot/agents/` |
| `copilot-cli` | settings | `.github/copilot/settings.json` | `~/.copilot/settings.json` |
| `copilot-cli` | settings | `.github/hooks/ai-rulez.json` | `~/.copilot/hooks/ai-rulez.json` |
| `copilot-cli` | settings | `.github/mcp.json` | `~/.copilot/mcp-config.json` |
| `cortex` | skills | `.cortex/skills/` | `~/.snowflake/cortex/skills/` |
| `cortex` | agents | `.cortex/agents/` | `~/.snowflake/cortex/agents/` |
| `cortex` | settings | `.cortex/settings.json` | `~/.snowflake/cortex/hooks.json` |
| `crush` | instructions | `CRUSH.md` | `~/.config/crush/CRUSH.md` |
| `crush` | skills | `.crush/skills/` | `~/.config/crush/skills/` |
| `crush` | settings | `crush.json` | `~/.config/crush/crush.json` |
| `cursor` | skills | `.agents/skills/` | `~/.agents/skills/` |
| `cursor` | agents | `.cursor/agents/` | `~/.cursor/agents/` |
| `cursor` | commands | `.cursor/commands/` | `~/.cursor/commands/` |
| `cursor` | settings | `.cursor/hooks.json` | `~/.cursor/hooks.json` |
| `cursor` | settings | `.cursor/mcp.json` | `~/.cursor/mcp.json` |
| `deepagents` | instructions | `AGENTS.md` | `~/.deepagents/agent/AGENTS.md` |
| `deepagents` | skills | `.deepagents/skills/` | `~/.deepagents/agent/skills/` |
| `deepagents` | agents | `.deepagents/agents/` | `~/.deepagents/agent/agents/` |
| `deepagents` | settings | `.deepagents/.mcp.json` | `~/.deepagents/.mcp.json` |
| `deepagents` | settings | `.deepagents/hooks.json` | `~/.deepagents/hooks.json` |
| `devin` | instructions | `AGENTS.md` | `~/.config/devin/AGENTS.md` |
| `devin` | skills | `.devin/skills/` | `~/.config/devin/skills/` |
| `devin` | agents | `.devin/agents/` | `~/.config/devin/agents/` |
| `devin` | settings | `.devin/config.json` | `~/.config/devin/config.json` |
| `devin` | settings | `.devin/mcp_config.json` | `~/.config/devin/mcp_config.json` |
| `dsh` | instructions | `AGENTS.md` | `~/.dsh/AGENTS.md` |
| `dsh` | skills | `.dsh/skills/` | `~/.dsh/skills/` |
| `factory` | instructions | `AGENTS.md` | `~/.factory/AGENTS.md` |
| `factory` | skills | `.factory/skills/` | `~/.factory/skills/` |
| `factory` | agents | `.factory/droids/` | `~/.factory/droids/` |
| `factory` | commands | `.factory/commands/` | `~/.factory/commands/` |
| `factory` | settings | `.factory/hooks.json` | `~/.factory/hooks.json` |
| `factory` | settings | `.factory/mcp.json` | `~/.factory/mcp.json` |
| `gemini` | instructions | `GEMINI.md` | `~/.gemini/GEMINI.md` |
| `gemini` | skills | `.agents/skills/` | `~/.agents/skills/` |
| `gemini` | agents | `.gemini/agents/` | `~/.gemini/agents/` |
| `gemini` | settings | `.gemini/settings.json` | `~/.gemini/settings.json` |
| `gitlab-duo` | instructions | `.gitlab/duo/chat-rules.md` | `~/.gitlab/duo/chat-rules.md` |
| `gitlab-duo` | commands | `.agents/commands/` | `~/.gitlab/duo/commands/` |
| `gitlab-duo` | settings | `.gitlab/duo/hooks.json` | `~/.gitlab/duo/hooks.json` |
| `gitlab-duo` | settings | `.gitlab/duo/mcp.json` | `~/.gitlab/duo/mcp.json` |
| `goose` | instructions | `.goosehints` | `~/.config/goose/.goosehints` |
| `goose` | skills | `.agents/skills/` | `~/.agents/skills/` |
| `goose` | agents | `.goose/agents/` | `~/.config/goose/agents/` |
| `goose` | settings | `.agents/plugins/ai-rulez/hooks/hooks.json` | `~/.agents/plugins/ai-rulez/hooks/hooks.json` |
| `grok` | instructions | `AGENTS.md` | `~/.grok/AGENTS.md` |
| `grok` | rules | `.grok/rules/` | `~/.grok/rules/` |
| `grok` | skills | `.grok/skills/` | `~/.grok/skills/` |
| `grok` | agents | `.grok/agents/` | `~/.grok/agents/` |
| `grok` | commands | `.grok/commands/` | `~/.grok/commands/` |
| `grok` | settings | `.grok/config.toml` | `~/.grok/config.toml` |
| `grok` | settings | `.grok/hooks/ai-rulez.json` | `~/.grok/hooks/ai-rulez.json` |
| `hermes` | skills | `.hermes/skills/` | `~/.hermes/skills/` |
| `hermes` | settings | `.hermes/config.yaml` | `~/.hermes/config.yaml` |
| `junie` | instructions | `AGENTS.md` | `~/.junie/AGENTS.md` |
| `junie` | skills | `.junie/skills/` | `~/.junie/skills/` |
| `junie` | agents | `.junie/agents/` | `~/.junie/agents/` |
| `junie` | commands | `.junie/commands/` | `~/.junie/commands/` |
| `junie` | settings | `.junie/config.json` | `~/.junie/config.json` |
| `junie` | settings | `.junie/mcp/mcp.json` | `~/.junie/mcp/mcp.json` |
| `kilo` | instructions | `AGENTS.md` | `~/.config/kilo/AGENTS.md` |
| `kilo` | rules | `.kilo/rules/` | `~/.kilo/rules/` |
| `kilo` | skills | `.kilo/skills/` | `~/.kilo/skills/` |
| `kilo` | agents | `.kilo/agents/` | `~/.config/kilo/agents/` |
| `kilo` | commands | `.kilo/commands/` | `~/.config/kilo/commands/` |
| `kilo` | settings | `.kilo/plugins/ai-rulez-hooks.js` | `~/.config/kilo/plugins/ai-rulez-hooks.js` |
| `kilo` | settings | `kilo.jsonc` | `~/.config/kilo/kilo.jsonc` |
| `kimi` | instructions | `AGENTS.md` | `~/.kimi-code/AGENTS.md` |
| `kimi` | skills | `.kimi-code/skills/` | `~/.kimi-code/skills/` |
| `kimi` | agents | `.kimi-code/agents/` | `~/.kimi-code/agents/` |
| `kimi` | settings | `.kimi-code/config.toml` | `~/.kimi-code/config.toml` |
| `kimi` | settings | `.kimi-code/mcp.json` | `~/.kimi-code/mcp.json` |
| `kiro` | instructions | `AGENTS.md` | `~/.kiro/steering/AGENTS.md` |
| `kiro` | rules | `.kiro/steering/` | `~/.kiro/steering/` |
| `kiro` | skills | `.kiro/skills/` | `~/.kiro/skills/` |
| `kiro` | agents | `.kiro/agents/` | `~/.kiro/agents/` |
| `kiro` | commands | `.kiro/prompts/` | `~/.kiro/prompts/` |
| `kiro` | settings | `.kiro/hooks/ai-rulez.json` | `~/.kiro/hooks/ai-rulez.json` |
| `kiro` | settings | `.kiro/settings/mcp.json` | `~/.kiro/settings/mcp.json` |
| `letta` | skills | `.agents/skills/` | `~/.letta/skills/` |
| `letta` | agents | `.letta/agents/` | `~/.letta/agents/` |
| `letta` | commands | `.commands/` | `~/.letta/commands/` |
| `letta` | settings | `.letta/settings.json` | `~/.letta/settings.json` |
| `mimocode` | instructions | `AGENTS.md` | `~/.config/mimocode/AGENTS.md` |
| `mimocode` | skills | `.mimocode/skills/` | `~/.config/mimocode/skills/` |
| `mimocode` | agents | `.mimocode/agents/` | `~/.config/mimocode/agents/` |
| `mimocode` | commands | `.mimocode/commands/` | `~/.config/mimocode/commands/` |
| `mimocode` | settings | `.mimocode/mimocode.jsonc` | `~/.config/mimocode/mimocode.jsonc` |
| `mimocode` | settings | `.mimocode/plugins/ai-rulez-hooks.js` | `~/.config/mimocode/plugins/ai-rulez-hooks.js` |
| `muse` | skills | `.agents/skills/` | `~/.config/muse/skills/` |
| `omp` | instructions | `AGENTS.md` | `~/.omp/agent/AGENTS.md` |
| `omp` | rules | `.omp/rules/` | `~/.omp/agent/rules/` |
| `omp` | skills | `.omp/skills/` | `~/.omp/agent/skills/` |
| `omp` | agents | `.omp/agents/` | `~/.omp/agent/agents/` |
| `omp` | commands | `.omp/commands/` | `~/.omp/agent/commands/` |
| `omp` | settings | `.omp/config.yml` | `~/.omp/agent/config.yml` |
| `omp` | settings | `.omp/mcp.json` | `~/.omp/agent/mcp.json` |
| `openclaw` | instructions | `AGENTS.md` | `~/.openclaw/workspace/AGENTS.md` |
| `opencode` | instructions | `AGENTS.md` | `~/.config/opencode/AGENTS.md` |
| `opencode` | skills | `.opencode/skills/` | `~/.config/opencode/skills/` |
| `opencode` | agents | `.opencode/agents/` | `~/.config/opencode/agents/` |
| `opencode` | commands | `.opencode/commands/` | `~/.config/opencode/commands/` |
| `opencode` | settings | `.opencode/plugins/ai-rulez-hooks.js` | `~/.config/opencode/plugins/ai-rulez-hooks.js` |
| `opencode` | settings | `opencode.json` | `~/.config/opencode/opencode.json` |
| `pi` | instructions | `AGENTS.md` | `~/.pi/agent/AGENTS.md` |
| `pi` | skills | `.agents/skills/` | `~/.pi/agent/skills/` |
| `pi` | agents | `.pi/agents/` | `~/.pi/agent/agents/` |
| `pi` | commands | `.pi/prompts/` | `~/.pi/agent/prompts/` |
| `pi` | settings | `.pi/extensions/ai-rulez-hooks.ts` | `~/.pi/agent/extensions/ai-rulez-hooks.ts` |
| `poolside` | instructions | `AGENTS.md` | `~/.config/poolside/AGENTS.md` |
| `poolside` | skills | `.poolside/skills/` | `~/.config/poolside/skills/` |
| `poolside` | settings | `.poolside/settings.yaml` | `~/.config/poolside/settings.yaml` |
| `qoder` | instructions | `AGENTS.md` | `~/.qoder/AGENTS.md` |
| `qoder` | rules | `.qoder/rules/` | `~/.qoder/rules/` |
| `qoder` | skills | `.qoder/skills/` | `~/.qoder/skills/` |
| `qoder` | agents | `.qoder/agents/` | `~/.qoder/agents/` |
| `qoder` | commands | `.qoder/commands/` | `~/.qoder/commands/` |
| `qoder` | settings | `.mcp.json` | `~/.qoder/settings.json` |
| `qoder` | settings | `.qoder/settings.json` | `~/.qoder/settings.json` |
| `qwen` | instructions | `QWEN.md` | `~/.qwen/QWEN.md` |
| `qwen` | rules | `.qwen/rules/` | `~/.qwen/rules/` |
| `qwen` | skills | `.qwen/skills/` | `~/.qwen/skills/` |
| `qwen` | agents | `.qwen/agents/` | `~/.qwen/agents/` |
| `qwen` | commands | `.qwen/commands/` | `~/.qwen/commands/` |
| `qwen` | settings | `.qwen/settings.json` | `~/.qwen/settings.json` |
| `reasonix` | instructions | `REASONIX.md` | `~/.reasonix/REASONIX.md` |
| `reasonix` | skills | `.reasonix/skills/` | `~/.reasonix/skills/` |
| `reasonix` | commands | `.reasonix/commands/` | `~/.reasonix/commands/` |
| `reasonix` | settings | `.reasonix/settings.json` | `~/.reasonix/settings.json` |
| `rovodev` | instructions | `AGENTS.md` | `~/.rovodev/AGENTS.md` |
| `rovodev` | skills | `.rovodev/skills/` | `~/.rovodev/skills/` |
| `rovodev` | agents | `.rovodev/subagents/` | `~/.rovodev/subagents/` |
| `takt` | rules | `.takt/facets/policies/` | `~/.takt/facets/policies/` |
| `takt` | skills | `.takt/facets/knowledge/` | `~/.takt/facets/knowledge/` |
| `takt` | agents | `.takt/facets/personas/` | `~/.takt/facets/personas/` |
| `takt` | commands | `.takt/facets/instructions/` | `~/.takt/facets/instructions/` |
| `trae` | skills | `.trae/skills/` | `~/.trae/skills/` |
| `vibe` | instructions | `AGENTS.md` | `~/.vibe/AGENTS.md` |
| `vibe` | skills | `.vibe/skills/` | `~/.vibe/skills/` |
| `vibe` | settings | `.vibe/config.toml` | `~/.vibe/config.toml` |
| `vibe` | settings | `.vibe/hooks.toml` | `~/.vibe/hooks.toml` |
| `warp` | instructions | `AGENTS.md` | `~/.agents/AGENTS.md` |
| `warp` | skills | `.warp/skills/` | `~/.warp/skills/` |
| `warp` | settings | `.warp/.mcp.json` | `~/.warp/.mcp.json` |
| `zcode` | instructions | `AGENTS.md` | `~/.zcode/AGENTS.md` |
| `zcode` | skills | `.zcode/skills/` | `~/.zcode/skills/` |
| `zcode` | agents | `.zcode/agents/` | `~/.zcode/agents/` |
| `zcode` | commands | `.zcode/commands/` | `~/.zcode/commands/` |
| `zcode` | settings | `.zcode/config.json` | `~/.zcode/cli/config.json` |
| `zed` | instructions | `.rules` | `~/.config/zed/AGENTS.md` |
| `zed` | skills | `.agents/skills/` | `~/.agents/skills/` |
| `zed` | settings | `.zed/settings.json` | `~/.config/zed/settings.json` |
| `zoocode` | instructions | `AGENTS.md` | `~/.roo/rules/AGENTS.md` |
| `zoocode` | skills | `.roo/skills/` | `~/.roo/skills/` |
| `zoocode` | commands | `.roo/commands/` | `~/.roo/commands/` |
<!-- user-scope-table:end -->

Presets not in the table have no documented user-level location for anything `--user` writes, and are
reported and skipped: `aiassistant`, `baz`, `codebuff` (only an MCP file), `replit` and `xum`.

Only an absolute home is used. `generate --user` refuses a relative `$HOME`.

### Relocated tool homes

A tool whose home directory an environment variable relocates follows it: when the variable is set to an
absolute path, every path in the tool's own home moves there. `CODEX_HOME=/data/codex` writes
`/data/codex/AGENTS.md` and `/data/codex/hooks.json`, while Codex's skills stay in the shared
`~/.agents/skills`. The variables are `CODEX_HOME`, `HERMES_HOME`, `DEEPAGENTS_HOME`, `KIMI_CODE_HOME`,
`QWEN_HOME`, `VIBE_HOME`, `DSH_HOME` and the others a preset's `[global] home_env` names. A relative value is
ignored with a warning and the default location is used. The files stay recorded in the manifest, so
`clean --user` removes them from the relocated directory, and the symlink and hand-authored-file rules
apply there too.

User-level MCP servers are not generated: Claude Code and the others keep them outside the files above.

## Project and user scope together

Both scopes are read by the harness, so the same skill name can load twice. `generate --user` warns about
both cases:

- **One harness reading several user directories** (reported for the presets you configure). OpenCode reads `~/.config/opencode/skills`,
  `~/.claude/skills` and `~/.agents/skills`; Cursor reads `~/.agents/skills`, `~/.cursor/skills`,
  `~/.claude/skills` and `~/.codex/skills`; Gemini CLI, Copilot and Codex read `~/.agents/skills` (plus their
  own). Generating skills for `claude` and `opencode` therefore installs each skill twice for OpenCode. Keep
  to presets that share a directory (`codex`, `gemini`, `cursor`, `pi` all use `~/.agents/skills`), or
  accept the duplicates.
- **A skill that also exists in the project you run from.** The vendor's documented precedence decides which
  copy runs:

| Harness | Same name at user and project level |
| ------- | ----------------------------------- |
| Claude Code | the user-level skill runs (personal over project) |
| Gemini CLI | the workspace skill runs (workspace over user) |
| Codex | both are listed; same-named skills are not merged or overridden |
| OpenCode, Cursor, Copilot, pi | no precedence is documented; keep names unique |

Instructions files are concatenated by the harnesses rather than overridden (Claude Code and Gemini CLI load
the user-level file and then the project's; Codex loads the global `AGENTS.md` first), so no warning is
needed for them.
