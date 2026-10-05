# User-level configuration

`ai-rulez generate` writes inside the project. `ai-rulez generate --user` does the same for a person: it
renders a user config into the per-user directories each harness reads (`~/.claude`, `~/.agents/skills`,
`~/.codex`, `~/.gemini`, `~/.config/opencode`, `~/.copilot`, `~/.pi/agent`), so personal or role-based
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
management. A preset with no documented user-level location (see the table) is reported and skipped.

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

- **Only documented locations.** An output is written only where the table below says a vendor documents it.
  Anything else a preset renders (commands, project rules folders, MCP files, `.claude/plugins.json`) is
  dropped and counted.
- **Hand-authored files are never replaced.** A file that exists and was not written by ai-rulez (not in the
  manifest, no generated banner, not byte-identical to the output) is skipped and listed as `skip:`. A skill
  directory holding one is skipped whole. `~/.claude/CLAUDE.md` you wrote yourself stays yours; ai-rulez
  writes nothing at that path.
- **Shared settings documents are merged, not rewritten.** `~/.claude/settings.json`, `~/.codex/hooks.json`,
  `~/.cursor/hooks.json` and `~/.gemini/settings.json` get only the keys ai-rulez owns (`hooks`,
  `permissions.*`, `env`, `skillOverrides` from [Hooks and permissions](settings.md)); every other key
  survives `generate` and `clean`.
- **No symlink escapes.** Every destination, resolved through symlinks, must stay inside the home directory.
  A `~/.claude` symlinked into `~/dotfiles` is followed; one pointing out of the home directory stops the
  run before anything is written. A symlinked file is skipped, except for the settings documents above.
- **Clean removes only what was recorded.** The manifest is `~/.config/ai-rulez/.generated-manifest.json`
  (plus `.generated-manifest.local.json` for the keys merged into shared documents). `clean --user`,
  and `generate --user` after you drop content, remove a file only if it is listed there and, for files
  that carry a generated header, still looks generated. Empty directories inside the owned roots
  (`~/.claude/skills/<id>`, ...) are pruned; `~/.claude`, `~/.codex` and the other harness homes are never
  removed, and the user config is never touched beyond its two manifest files. Keep `~/.config/ai-rulez` in your dotfiles repository if you
  like, but add the two manifest files to its ignore list.
- **`--dry-run` writes nothing**, not even the manifest.

## Where things go

Verified against vendor documentation on 2026-10-04 (the `Verified` column); a test checks this table
against the code.

| Harness | Output | Project path | User-level path | Verified |
| ------- | ------ | ------------ | --------------- | -------- |
| `claude` | instructions | `CLAUDE.md` | `~/.claude/CLAUDE.md` | 2026-10-04 |
| `claude` | rules | `.claude/rules/` | `~/.claude/rules/` | 2026-10-04 |
| `claude` | skills | `.claude/skills/` | `~/.claude/skills/` | 2026-10-04 |
| `claude` | agents | `.claude/agents/` | `~/.claude/agents/` | 2026-10-04 |
| `claude` | settings | `.claude/settings.json` | `~/.claude/settings.json` | 2026-10-04 |
| `codex` | instructions | `AGENTS.md` | `~/.codex/AGENTS.md` | 2026-10-04 |
| `codex` | skills | `.agents/skills/` | `~/.agents/skills/` | 2026-10-04 |
| `codex` | settings | `.codex/hooks.json` | `~/.codex/hooks.json` | 2026-10-04 |
| `gemini` | instructions | `GEMINI.md` | `~/.gemini/GEMINI.md` | 2026-10-04 |
| `gemini` | skills | `.agents/skills/` | `~/.agents/skills/` | 2026-10-04 |
| `gemini` | agents | `.gemini/agents/` | `~/.gemini/agents/` | 2026-10-04 |
| `gemini` | settings | `.gemini/settings.json` | `~/.gemini/settings.json` | 2026-10-04 |
| `opencode` | instructions | `AGENTS.md` | `~/.config/opencode/AGENTS.md` | 2026-10-04 |
| `opencode` | skills | `.opencode/skills/` | `~/.config/opencode/skills/` | 2026-10-04 |
| `opencode` | agents | `.opencode/agents/` | `~/.config/opencode/agents/` | 2026-10-04 |
| `cursor` | skills | `.agents/skills/` | `~/.agents/skills/` | 2026-10-04 |
| `cursor` | settings | `.cursor/hooks.json` | `~/.cursor/hooks.json` | 2026-10-04 |
| `copilot` | skills | `.github/skills/` | `~/.copilot/skills/` | 2026-10-04 |
| `copilot` | settings | `.github/hooks/ai-rulez.json` | `~/.copilot/hooks/ai-rulez.json` | 2026-10-04 |
| `pi` | instructions | `AGENTS.md` | `~/.pi/agent/AGENTS.md` | 2026-10-04 |
| `pi` | skills | `.agents/skills/` | `~/.pi/agent/skills/` | 2026-10-04 |

Sources: Claude Code `/docs/en/memory`, `/skills`, `/sub-agents`, `/settings`; Codex
`learn.chatgpt.com/docs/agent-configuration/agents-md`, `/docs/build-skills`, `/docs/hooks`; Gemini CLI
`/docs/cli/gemini-md/`, `/docs/cli/skills/`, `/docs/core/subagents/`, `/docs/hooks/`; OpenCode
`/docs/rules/`, `/docs/skills/`, `/docs/agents/`; Cursor `/docs/context/skills`, `/docs/hooks`; Copilot
`add-skills` and `hooks-configuration`; pi `/docs/latest/configuration`.

Not written, because no vendor page documents a user-level file for them: Cursor rules, commands and agents
(Cursor keeps user rules outside the file system), Codex agents and commands, Copilot instructions and
agents, Amp, and every preset not listed (`devin`, `junie`, `cline`, `continue-dev`, `antigravity`,
`hermes`, `baz`, `xum`). Codex's `CODEX_HOME`, pi's `PI_CODING_AGENT_DIR` and Copilot's `COPILOT_HOME` are
not honoured: the default locations are used.

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
