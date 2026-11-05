# AGENTS.md and .agents/skills

`AGENTS.md` and `.agents/skills/` are the cross-tool conventions for project instructions and
[Agent Skills](https://agentskills.io). With `agents_md = true` (the default), ai-rulez writes them once and every
preset that reads them stops writing its own copy: Codex, Cursor, Copilot, Gemini CLI, Devin and the others read the
shared files, and `CLAUDE.md` becomes a one-line import of `AGENTS.md`.

```toml
agents_md = true    # the default
# agents_md = false # every preset writes its own files
```

To keep `AGENTS.md` as your own source file instead of ai-rulez output, see [.agents as a source](agents-source.md).
To write `AGENTS.md` or `CLAUDE.md` by hand and keep ai-rulez away from them, use
[`skip_outputs`](configuration.md#skip_outputs) (`"agents_md"`, `"claude_md"` or `"instructions"`).

## Example

```toml
version = "5.0"
name = "acme"
presets = ["claude", "codex", "cursor", "gemini", "copilot"]
```

With an always-on rule `security`, a rule `go-style` with `paths: ["**/*.go"]`, a skill `release` and an agent
`reviewer`, `generate` writes:

```text
AGENTS.md                                     # security, and go-style with an "Applies to" line
CLAUDE.md                                     # banner + @AGENTS.md
.agents/skills/release/SKILL.md               # shared skills tree
.claude/skills/release/SKILL.md               # Claude Code does not read .agents/skills
.claude/rules/go-style.md
.cursor/rules/go-style.mdc
.github/instructions/go-style.instructions.md
.gemini/settings.json                         # context.fileName: ["AGENTS.md", "GEMINI.local.md"]
.claude/agents/reviewer.md
.codex/agents/reviewer.toml
.cursor/agents/reviewer.md
.gemini/agents/reviewer.md
.github/agents/reviewer.agent.md
.mcp.json, .cursor/mcp.json, .vscode/mcp.json, .codex/config.toml, .claude/settings.json
```

No `GEMINI.md`, no `.github/copilot-instructions.md`, no `.github/skills/` and no `.claude/rules/security.md`: those
tools get the content from `AGENTS.md` and `.agents/skills`. With `agents_md = false` the same project writes 24
files instead of 18.

```markdown
<!-- CLAUDE.md, after the generated banner -->
@AGENTS.md
```

## What changes

- Always-on rules and context go into one `AGENTS.md`, plus `<scope>/AGENTS.md` for each [`[[scopes]]`](monorepo.md#scopes)
  entry.
- Skills without `targets` go into one `.agents/skills/<name>/SKILL.md` tree (see [Skills](#skills)).
- Presets that read the shared files stop writing their own root file (`GEMINI.md`, `.hermes.md`,
  `.github/copilot-instructions.md`, ...), their copy of `AGENTS.md` and their own skills folder.
- Commands that `codex`, `antigravity`, `devin` and `warp` run as skills go to the shared tree as user-invocable
  skills (`disable-model-invocation: true`, plus `agents/openai.yaml` for Codex), unless a skill has the same id. A
  command with `targets` stays in each targeted preset's own skills folder.
- Everything else stays per preset: path-scoped rule folders, agents, other commands, MCP files and settings.

## Tool support

`V` verified against official docs or source, `P` partial or experimental, `N` not supported, `?` not verified. The
last two rows are tools that matter because they can shadow `AGENTS.md`. Tool behavior changes quickly; re-check a
cell before relying on it.

| Tool                       | Root `AGENTS.md`                                                  | Nested `AGENTS.md`                                          | Precedence and double-loading                                                                                                                                  | `.agents/rules` | `.agents/skills`                                                     | Agents, commands                                   |
| -------------------------- | ----------------------------------------------------------------- | ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------- | -------------------------------------------------------------------- | -------------------------------------------------- |
| Claude Code                | P, recent versions only, and only when no `CLAUDE.md`, `.claude/CLAUDE.md` or `CLAUDE.local.md` exists in cwd or an ancestor | V lazily, when a file in a subdirectory without any `CLAUDE.md` is read | `@AGENTS.md` import in `CLAUDE.md` never loads twice. `CLAUDE.local.md` suppresses `AGENTS.md` unless the user-level `instructionFiles` setting is `claude-md-and-agents-md`. Reads `.claude/AGENTS.md`, not `AGENTS.local.md` | N               | N (`.claude/skills`)                                                 | N (`.claude/agents`, `.claude/commands`)           |
| Codex CLI                  | V                                                                 | V project root down to cwd only; no lazy loading below cwd  | Per directory first of `AGENTS.override.md`, `AGENTS.md`, fallback names; concatenated root-down; `project_doc_max_bytes` 32 KiB                               | N               | V repo-local, cwd up to repo root; `~/.agents/skills`                | ? (`.codex/agents/*.toml`)                         |
| Cursor                     | V                                                                 | V any subdirectory, more specific wins                      | ? Docs call `AGENTS.md` an alternative to `.cursor/rules`; co-presence and deduplication not documented                                                         | N               | V `.agents/skills`, `.cursor/skills`, nested ones anywhere           | V `.cursor/agents`, `.claude/agents`, `.codex/agents`; no `.agents/agents` |
| Copilot cloud agent, CLI   | V                                                                 | V nearest file in the tree takes precedence                 | Cloud agent: all relevant instruction sets are provided, no deduplication. CLI: removes duplicate copies of identical instructions, defines no general precedence | N               | V `.github/skills`, `.claude/skills`, `.agents/skills`               | ? (`.github/agents`)                               |
| Copilot in VS Code         | V chat and agent                                                  | P experimental `chat.useNestedAgentsMdFiles`                | Additive. `CLAUDE.md` needs `chat.useClaudeMdFile`                                                                                                              | N               | V same three locations                                               | ?                                                  |
| Devin                      | V always-on rule; `agents.md` also recognized                     | V subdirectory file is a glob rule `<dir>/**`               | Same rules engine as `.devin/rules`                                                                                                                            | N               | V `.agents/skills`, native `.devin/skills`                          | ? workflows                                        |
| Gemini CLI                 | N by default: reads only `GEMINI.md` unless `context.fileName` lists more | V for configured names, hierarchical and on file access | Only names in `context.fileName` load (`.gemini/settings.json`); `@file.md` imports work in `GEMINI.md`                                                          | N               | V `.agents/skills` beats `.gemini/skills` within a tier              | N (`.gemini/agents`, `.gemini/commands` only)      |
| Antigravity                | V                                                                 | V `AGENTS.md`, `GEMINI.md` or `.agents/rules/` in any subdirectory | Cumulative; directory level wins on conflict. 24 KB per file, 20k tokens in aggregate                                                                          | V any directory level, immediate `.md` children only, `trigger` frontmatter | V `<workspace>/.agents/skills`; legacy `.agent/skills`               | ? workflows are being superseded by skills         |
| Junie                      | V                                                                 | ? not documented                                            | `.junie/AGENTS.md` first; else `AGENTS.md` + `.junie/rules/*.md`; else legacy `.junie/guidelines.md`; identical content deduplicated                           | N               | V `.junie/skills`, `.agents/skills`                                  | ?                                                  |
| Cline                      | V                                                                 | ? shipped code reads the root file only                     | Listed beside `.clinerules`, `.cursorrules`; per-file toggles                                                                                                  | N               | V in code (`.agents/skills`); docs list only `.cline/skills` and others | N                                                  |
| opencode                   | V                                                                 | V lazily, nearest file per read                             | Finds the first existing of `AGENTS.md`, `CLAUDE.md`, `CONTEXT.md` and stacks every ancestor copy of it                                                         | N               | V `.agents/skills`, `.claude/skills`, `.opencode/skills`             | N (`.opencode`)                                    |
| Amp                        | V                                                                 | V when the agent reads a file in the subtree                | Per directory `AGENTS.md`, else `AGENT.md`, else `CLAUDE.md`                                                                                                    | N               | V `.agents/skills` in project and parents                            | ?                                                  |
| xum                        | V                                                                 | V                                                           | `AGENTS.md` > `AGENT.md` > `CLAUDE.md`; also `AGENTS.local.md`                                                                                                  | N               | ?                                                                    | ?                                                  |
| pi                         | V                                                                 | V                                                           | Reads `AGENTS.md`; skills from `.agents/skills` (preferred) or `.pi/skills`; MCP in `.pi/mcp.json`                                                            | N               | V `.agents/skills` beats `.pi/skills`                                | ? (`.pi/agents` subagents extension)               |
| Hermes                     | V git root to cwd                                                 | V                                                           | One context type only: `.hermes.md` shadows `AGENTS.md` entirely                                                                                                | N               | V `.hermes/skills`, `.agents/skills`                                 | ?                                                  |
| Zed                        | V                                                                 | N                                                           | One file per worktree root: the first existing of `.rules`, `.cursorrules`, `.devin/rules`, `.clinerules`, `.github/copilot-instructions.md`, `AGENT.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`. Earlier entries shadow `AGENTS.md` | N               | V `.agents/skills`                                                   | N                                                  |
| Warp                       | V file name must be upper case                                    | V root and current directory                                | `WARP.md` beats `AGENTS.md` in the same directory                                                                                                               | N               | V `.agents/skills` (recommended)                                     | ?                                                  |

Only Antigravity reads `.agents/rules`, so ai-rulez writes it for that preset alone.

## What each preset does

Rules mode is the default `split` unless noted. "Shared" means the one `AGENTS.md` or `.agents/skills` tree.

| Preset                         | Reads AGENTS.md                      | Root file                                   | Skills                                         | Written beside the shared files                                                                                                  |
| ------------------------------ | ------------------------------------ | ------------------------------------------- | ---------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `codex`, `opencode`, `xum`, `pi` | natively                             | shared `AGENTS.md`                          | shared; `.opencode/skills`, `.xum/skills` dropped (`pi` never writes skills under `.pi`) | their own agents, commands and MCP files                                                                                         |
| `amp`                          | natively                             | shared `AGENTS.md`                          | shared                                         | unchanged                                                                                                                        |
| `claude`                       | through `CLAUDE.md` containing `@AGENTS.md` | `CLAUDE.md` is a banner plus `@AGENTS.md` | `.claude/skills` kept (Claude ignores `.agents/skills`) | `.claude/rules/*.md` for non-always-on items, `.claude/agents`, `.mcp.json` (MCP servers), `.claude/settings.json` (only hooks, permissions and managed keys)                              |
| `gemini`                       | through `context.fileName` in `.gemini/settings.json` | `GEMINI.md` not written       | shared                                         | `.gemini/settings.json`, `.gemini/agents/<id>.md`, `.mcp.json`                                                                   |
| `antigravity`                  | natively                             | `GEMINI.md` not written                     | shared                                         | `.agents/rules/*.md` for non-always-on items, `.agents/agents/<id>.md`, `.agents/mcp_config.json`, `.mcp.json`                     |
| `hermes`                       | natively                             | `.hermes.md` not written                    | shared                                         | `.mcp.json`                                                                                                                      |
| `cursor`                       | natively                             | none                                        | shared                                         | `.cursor/rules/*.mdc` for non-always-on items and glob context, `.cursor/agents/<id>.md`, `.mcp.json`                            |
| `copilot`                      | natively                             | `.github/copilot-instructions.md` not written | shared; `.github/skills` dropped             | `.github/instructions/*.instructions.md` for `applyTo`-scoped items, `.github/agents/<id>.agent.md`                              |
| `devin`                        | natively                             | none                                        | shared; `.devin/skills` dropped                | `.devin/rules/*.md` for non-always-on items and glob context, `.devin/agents/<id>.md`                                           |
| `cline`                        | natively                             | none                                        | shared; `.cline/skills` dropped                | `.clinerules/*.md` for non-always-on items and glob context, `.cline/agents/<id>.yaml`                                             |
| `junie`                        | natively                             | shared `AGENTS.md`                          | shared; `.junie/skills` dropped                | `.junie/rules/*.md` for non-always-on items, `.junie/agents/<id>.md`                                                             |

Every preset still writes its MCP file. Custom presets and provider specs not in the table take no part; their
`AGENTS.md` or `.agents/skills` output is dropped in favor of the shared copy (see
[Overlapping writers](#overlapping-writers)).

### How each tool finds AGENTS.md

- **Native readers** (`codex`, `opencode`, `xum`, `amp`, `pi`, `hermes`, `cursor`, `copilot`, `devin`, `cline`,
  `junie`, `antigravity`): nothing to configure.
- **Claude Code** reads `CLAUDE.md` first and reads `AGENTS.md` itself only in recent versions, and only when no
  `CLAUDE.md` exists. ai-rulez therefore writes `CLAUDE.md` as the banner plus `@AGENTS.md`, which works on every
  version and never loads the file twice.
- **Gemini CLI** loads only the names in `context.fileName`. ai-rulez merges `"AGENTS.md"` and `"GEMINI.local.md"`
  into `context.fileName` in `.gemini/settings.json`, keeping your other names and keys, and writes the file even
  without MCP servers. Gemini then no longer loads `GEMINI.md`, so it is not written.

### Files that would shadow AGENTS.md

Some tools load a single instruction file chosen by order, so a preset's own root file would hide `AGENTS.md`:

- **Zed** takes the first existing of `.rules`, `.cursorrules`, `.devin/rules`, `.clinerules`,
  `.github/copilot-instructions.md`, `AGENT.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`. The `copilot` preset
  therefore does not write `.github/copilot-instructions.md`.
- **Hermes** loads one context type, and `.hermes.md` would shadow `AGENTS.md`; the `hermes` preset does not write it.
- **Warp** prefers `WARP.md` in the same directory. No preset writes it; do not add one by hand.
- **Junie** prefers `.junie/AGENTS.md`; otherwise it reads the root `AGENTS.md` with `.junie/rules`. The `junie`
  preset writes the root `AGENTS.md`.

### Overlapping writers

If another preset, including a custom provider, also writes `AGENTS.md` or a file under `.agents/skills`, the shared
output wins and the other copy is not written.

## Where rules and context go

The shared `AGENTS.md` always carries always-on rules and context, and items scoped only by negated globs
(`!gen/**`), which no rules folder can express. Scoped, auto and manual items depend on the presets that rely on the
file:

| Kind | Presets with a rules folder (`cursor`, `devin`, `cline`; `claude`, `antigravity`, `junie` in split mode) | Presets without one (`codex`, `opencode`, `amp`, `xum`, `pi`, `hermes`, `gemini`) |
| --- | --- | --- |
| Always-on rule or context | `AGENTS.md` only | `AGENTS.md` |
| Glob-scoped rule or context | The preset's folder | `AGENTS.md`, with an `_Applies to_` line |
| Auto rule | The folder | `AGENTS.md`, with a `_When relevant_` line |
| Manual rule | The folder | `AGENTS.md` |
| Only negated globs | `AGENTS.md` only | `AGENTS.md` |

The decision is made once for the whole file; the most inclusive row among the configured presets wins:

| Configured preset (rules mode) | Scoped items in `AGENTS.md` | Auto and manual items in `AGENTS.md` |
| --- | --- | --- |
| `codex`, `opencode`, `amp`, `xum`, `pi`, `hermes`, `gemini` | yes | yes |
| `claude`, `antigravity` (split) | no | no |
| `claude`, `antigravity` (inline) | no | yes |
| `cursor`, `devin`, `cline` (either) | no | no |
| `copilot` (either) | no | yes (its folder holds only `applyTo` items) |
| `junie` (split) | no | no |
| `junie` (inline) | yes (it writes no rule files) | yes |

### Duplication trade-off

When a preset without a rules folder is configured next to one with a folder (`claude` + `codex`, `cursor` +
`gemini`, ...), scoped items appear in `AGENTS.md` and in the folder, so the tool with the folder loads them twice.
`generate` warns:

```text
WARN  AGENTS.md inlines glob-scoped rules and context for presets without a rules folder; presets with one also read it,
      so those items load in every session. Set [rules] scoped_fallback = "skill" to serve them as path-gated skills
```

Options:

- set `[rules] scoped_fallback = "skill"` to serve scoped items as path-gated skills in `.agents/skills` instead of
  inlining them (see [Glob-scoped rules and AGENTS.md](rules.md#glob-scoped-rules-and-agentsmd));
- keep to presets that all have rules folders;
- accept the overlap, or set `agents_md = false`.

Machine-local rules are never part of the shared `AGENTS.md`; see [Scopes, local files and hashes](#scopes-local-files-and-hashes).

## Skills

- Skills without `targets` are written once to `.agents/skills/<name>/SKILL.md`, with their bundled files, in the
  generic Agent Skills format (`name`, `description`).
- The tree follows each skill's [`delivery`](skills.md#dynamic-loading-delivery): `served` skills are left out and
  the `dynamic-skills` stub is added; `static` and `both` skills are written. If a preset reading the tree cannot call
  MCP tools (`xum`, `hermes`, ...), the served skills stay in the tree.
- Claude Code reads `.claude/skills`, not `.agents/skills`, so the `claude` preset keeps its own copy.
- Skills with `targets` stay out of the shared tree and go to the per-preset folders the targets allow. A skill
  targeting `codex` goes to Codex's folder, which is `.agents/skills` (or `codex_skills_dir`), so other readers of
  `.agents/skills` see it too.

## targets

Frontmatter `targets` select the presets or files an item is written for. Because `AGENTS.md` is shared:

- A rule or context item is in `AGENTS.md` when its `targets` name a preset that relies on the file (`claude`,
  `gemini`, `cursor`, `codex`, ...), a root file such a preset replaced (`CLAUDE.md`, `GEMINI.md`, `.hermes.md`,
  `.github/copilot-instructions.md`), or a default owner of `AGENTS.md` (`codex`, `opencode`, `xum`, `amp`, `pi`,
  `junie`), configured or not.
- An always-on rule targeted at one preset reaches every tool that reads `AGENTS.md`.
- To limit an always-on item to one tool, target that tool's rules folder (`.cursor/rules/`, `.claude/rules/`,
  `.github/instructions/`, ...) instead of its preset name: it is then written as a rule file there and kept out of
  `AGENTS.md`.

## Scopes, local files and hashes

**Scopes.** Each `[[scopes]]` entry gets `<scope>/AGENTS.md` with only the scope's own content, and the scope's
`claude` preset writes `<scope>/CLAUDE.md` as an `@AGENTS.md` import. `.gemini/settings.json` is written only at the
root, which is what makes Gemini find nested `AGENTS.md` files; if `gemini` is configured for a scope but not for the
root, `generate` warns.

**Local content.** Machine-local rules and context never enter the shared `AGENTS.md`. Each tool gets them through its
own local file (`CLAUDE.local.md`, `AGENTS.local.md`, `AGENTS.override.md` for Codex, ...); the per-preset list is in
[Local configuration](local-overrides.md#generated-output).

**Hashes.** `AGENTS.md` and the files under `.agents/skills` share one `Content-Hash` computed from their content and
the settings that shape them, not from the list of presets. Adding a preset that reads the file leaves the hash
unchanged unless the file itself changes.

## Switching the flag

Edit `agents_md` and run `ai-rulez generate`:

- **On:** files only the per-tool layout needed (`GEMINI.md`, `.hermes.md`, `.github/copilot-instructions.md`,
  `.devin/skills`, ...) are removed through the manifest, and `CLAUDE.md` is rewritten as the import.
- **Off:** the per-tool files are written again, and shared files that no preset writes itself are removed.
- **`.gemini/settings.json`:** on switching off, a `context.fileName` ai-rulez wrote is set back to
  `["GEMINI.md", "GEMINI.local.md"]`; in a list you wrote, only the `AGENTS.md` entry ai-rulez added is removed.
- **Hand-written files** are never removed: only files in the manifest are cleaned up.

## Caveats

- **Duplication** is possible in mixed setups (see the [trade-off](#duplication-trade-off)). The Copilot cloud
  agent and Copilot in VS Code document no deduplication; Copilot CLI removes only identical copies.
- **Size limits** apply to the one larger file (Codex reads 32 KiB of project instructions by default); see
  [Size limits](rules.md#size-limits) for the limits and how to shrink the file.
- **Nested `AGENTS.md`** support varies: Codex reads only from the project root down to the working directory;
  Cursor, Copilot, Amp and opencode load nested files; Zed does not.
- **Not verified:** the `?` cells above, including whether the tools read `.codex/agents` and `.github/agents`,
  which `generate` writes, and whether Copilot follows the `@AGENTS.md` import in `CLAUDE.md`.

See also: [`agents_md`](configuration.md#agents_md), [Rules](rules.md), [Local configuration](local-overrides.md),
[Monorepo](monorepo.md).
