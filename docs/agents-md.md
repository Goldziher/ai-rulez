# AGENTS.md and .agents/skills

`AGENTS.md` and `.agents/skills/` are the closest thing to a cross-tool convention for project instructions and
Agent Skills. With `agents_md = true`, ai-rulez renders them once and every preset that reads them stops writing its
own copy. The flag is off by default; with it off, output is byte-for-byte what earlier releases produced.

```toml
agents_md = true
```

What changes:

- Always-on rules and context go into one `AGENTS.md` (plus a nested `<scope>/AGENTS.md` per `[[scopes]]` entry).
- Skills without `targets` go into one `.agents/skills/<name>/SKILL.md` tree.
- Presets that read these files stop writing their root file (`GEMINI.md`, `.hermes.md`, ...), their copy of
  `AGENTS.md` and their own skills directory.
- Everything the tool cannot read from the shared files stays per-preset: scoped rules folders, agents, commands,
  MCP files and settings. A rules folder is created only when at least one rule file is written into it.

Without the flag, `codex`, `opencode`, `xum`, `pi`, `amp` and `baz` already write the same `AGENTS.md`, while `claude`, `gemini`,
`cursor` and the rest each repeat the same content in their own file.

## Tool support

Research as of 2026-10-03. `V` verified against official docs or source, `P` partial or experimental, `N` not
supported, `?` not verified. Tools are listed in the order of the presets that read the shared files, followed by two
tools that only matter because they can shadow `AGENTS.md`.

| Tool                       | Root `AGENTS.md`                                                  | Nested `AGENTS.md`                                          | Precedence and double-loading                                                                                                                                  | `.agents/rules` | `.agents/skills`                                                     | Agents, commands                                   |
| -------------------------- | ----------------------------------------------------------------- | ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------- | -------------------------------------------------------------------- | -------------------------------------------------- |
| Claude Code                | V, only when no `CLAUDE.md`, `.claude/CLAUDE.md` or `CLAUDE.local.md` exists in cwd or an ancestor (v2.1.277+; all sessions v2.1.281+) | V lazily, when a file in a subdirectory without any `CLAUDE.md` is read | `@AGENTS.md` import in `CLAUDE.md` never loads twice. `CLAUDE.local.md` suppresses `AGENTS.md` unless the user-level `instructionFiles` setting is `claude-md-and-agents-md`. Reads `.claude/AGENTS.md`, not `AGENTS.local.md` | N               | N (`.claude/skills`)                                                 | N (`.claude/agents`, `.claude/commands`)           |
| Codex CLI                  | V                                                                 | V project root down to cwd only; no lazy loading below cwd  | Per directory first of `AGENTS.override.md`, `AGENTS.md`, fallback names; concatenated root-down; `project_doc_max_bytes` 32 KiB                               | N               | V repo-local, cwd up to repo root; `~/.agents/skills`                | ? (`.codex/agents/*.toml`)                         |
| Cursor                     | V                                                                 | V any subdirectory, more specific wins                      | ? Docs call `AGENTS.md` an alternative to `.cursor/rules`; co-presence and deduplication not documented                                                         | N               | V `.agents/skills`, `.cursor/skills`, nested ones anywhere           | V `.cursor/agents`, `.claude/agents`, `.codex/agents`; no `.agents/agents` |
| Copilot cloud agent, CLI   | V                                                                 | V nearest file in the tree takes precedence                 | Cloud agent: all relevant instruction sets are provided, no deduplication. CLI: removes duplicate copies of identical instructions, defines no general precedence | N               | V `.github/skills`, `.claude/skills`, `.agents/skills`               | ? (`.github/agents`)                               |
| Copilot in VS Code         | V chat and agent                                                  | P experimental `chat.useNestedAgentsMdFiles`                | Additive. `CLAUDE.md` needs `chat.useClaudeMdFile`                                                                                                              | N               | V same three locations                                               | ?                                                  |
| Devin                      | V always-on rule; `agents.md` also recognized                     | V subdirectory file is a glob rule `<dir>/**`               | Same rules engine as `.devin/rules`                                                                                                                            | N               | V `.agents/skills`, native `.devin/skills`                          | ? workflows                                        |
| Gemini CLI                 | N by default: reads only `GEMINI.md` unless `context.fileName` lists more | V for configured names, hierarchical and on file access | Only names in `context.fileName` load (`.gemini/settings.json`); `@file.md` imports work in `GEMINI.md`                                                          | N               | V `.agents/skills` beats `.gemini/skills` within a tier              | N (`.gemini/agents`, `.gemini/commands` only)      |
| Antigravity                | V                                                                 | V `AGENTS.md`, `GEMINI.md` or `.agents/rules/` in any subdirectory | Cumulative; directory level wins on conflict. 24 KB per file, 20k tokens in aggregate                                                                          | V any directory level, immediate `.md` children only, `trigger` frontmatter | V `<workspace>/.agents/skills`; legacy `.agent/skills`               | ? workflows are being superseded by skills         |
| Junie                      | V                                                                 | ? not in the fetched docs                                   | `.junie/AGENTS.md` first; else `AGENTS.md` + `.junie/rules/*.md`; else legacy `.junie/guidelines.md`; identical content deduplicated                           | N               | V `.junie/skills`, `.agents/skills`                                  | ?                                                  |
| Cline                      | V                                                                 | ? shipped code reads the root file only                     | Listed beside `.clinerules`, `.cursorrules`; per-file toggles                                                                                                  | N               | V in code (`.agents/skills`); docs list only `.cline/skills` and others | N                                                  |
| opencode                   | V                                                                 | V lazily, nearest file per read                             | Finds the first existing of `AGENTS.md`, `CLAUDE.md`, `CONTEXT.md` and stacks every ancestor copy of it                                                         | N               | V `.agents/skills`, `.claude/skills`, `.opencode/skills`             | N (`.opencode`)                                    |
| Amp                        | V                                                                 | V when the agent reads a file in the subtree                | Per directory `AGENTS.md`, else `AGENT.md`, else `CLAUDE.md`                                                                                                    | N               | V `.agents/skills` in project and parents                            | ?                                                  |
| xum                        | V                                                                 | V                                                           | `AGENTS.md` > `AGENT.md` > `CLAUDE.md`; also `AGENTS.local.md`                                                                                                  | N               | ?                                                                    | ?                                                  |
| pi                         | V                                                                 | V                                                           | Reads `AGENTS.md`; skills from `.agents/skills` (preferred) or `.pi/skills`; MCP in `.pi/mcp.json`                                                            | N               | V `.agents/skills` beats `.pi/skills`                                | ? (`.pi/agents` subagents extension)               |
| Hermes                     | V git root to cwd                                                 | V                                                           | One context type only: `.hermes.md` shadows `AGENTS.md` entirely                                                                                                | N               | V `.hermes/skills`, `.agents/skills`                                 | ?                                                  |
| Zed                        | V                                                                 | N                                                           | One file per worktree root: the first existing of `.rules`, `.cursorrules`, `.devin/rules`, `.clinerules`, `.github/copilot-instructions.md`, `AGENT.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`. Earlier entries shadow `AGENTS.md` | N               | V `.agents/skills`                                                   | N                                                  |
| Warp                       | V file name must be upper case                                    | V root and current directory                                | `WARP.md` beats `AGENTS.md` in the same directory                                                                                                               | N               | V `.agents/skills` (recommended)                                     | ?                                                  |

Only Antigravity reads `.agents/rules`; ai-rulez uses it for that tool alone. Other tools covered in the research
(Kilo Code, Roo Code, Factory, Kimi CLI, Aider) are not ai-rulez presets and are omitted.

Sources, all fetched 2026-10-03: Claude Code memory docs (`agents-md` section) and release notes for v2.1.277;
Cursor docs (`cursor.com/docs/context/rules`, `cursor.com/docs/context/skills`); GitHub docs on repository
instructions and agent skills; the VS Code custom-instructions matrix; Devin docs (`docs.devin.ai`, AGENTS.md and
skills); Gemini CLI docs and source (`settingsSchema.ts`, `memoryTool.ts`); the Antigravity rules and skills pages;
Junie guidelines and agent-skills docs; Cline rules docs and extension source; Amp docs
(`ampcode.com/docs/customize/agents-md`); and source reads of Codex (`agents_md.rs`, skills host roots), opencode
(`instruction.ts`), Hermes (`prompt_builder.py`) and Zed (`prompts.rs`, `agent.rs`). Tools change quickly; re-check a
cell before relying on it.

## What each preset does

Rules mode is the default `split` unless noted. "Shared" means the one `AGENTS.md` or `.agents/skills` tree.
Paths below are what a project with an always-on rule, an overview context file, a glob rule, auto and manual
rules, a glob-scoped context file, two skills, one agent and one MCP server writes (taken from the test fixtures).

| Preset                         | Reads AGENTS.md                      | Root file                                   | Skills                                         | Written beside the shared files                                                                                                  |
| ------------------------------ | ------------------------------------ | ------------------------------------------- | ---------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `codex`, `opencode`, `xum`, `pi` | natively                             | shared `AGENTS.md`                          | shared; `.opencode/skills`, `.xum/skills` dropped (`pi` never writes skills under `.pi`) | their own agents, commands and MCP files                                                                                         |
| `amp`                          | natively                             | shared `AGENTS.md`                          | shared                                         | unchanged                                                                                                                        |
| `claude`                       | through `CLAUDE.md` containing `@AGENTS.md` | `CLAUDE.md` is a banner plus `@AGENTS.md` | `.claude/skills` kept (Claude ignores `.agents/skills`) | `.claude/rules/*.md` for non-always-on items, `.claude/agents`, `.claude/settings.json`, `.mcp.json`                              |
| `gemini`                       | through `context.fileName` in `.gemini/settings.json` | `GEMINI.md` not written       | shared                                         | `.gemini/settings.json`, `.gemini/agents/<id>.md`, `.mcp.json`                                                                   |
| `antigravity`                  | natively                             | `GEMINI.md` not written                     | shared                                         | `.agents/rules/*.md` for non-always-on items, `.agents/agents/<id>.md`, `.agents/mcp_config.json`, `.mcp.json`                     |
| `hermes`                       | natively                             | `.hermes.md` not written                    | shared                                         | `.mcp.json`                                                                                                                      |
| `cursor`                       | natively                             | none                                        | shared                                         | `.cursor/rules/*.mdc` for non-always-on items and glob context, `.cursor/agents/<id>.md`, `.mcp.json`                            |
| `copilot`                      | natively                             | `.github/copilot-instructions.md` not written | shared; `.github/skills` dropped             | `.github/instructions/*.instructions.md` for `applyTo`-scoped items, `.github/agents/<id>.agent.md`                              |
| `devin`                        | natively                             | none                                        | shared; `.devin/skills` dropped                | `.devin/rules/*.md` for non-always-on items and glob context, `.devin/agents/<id>.md`                                           |
| `cline`                        | natively                             | none                                        | shared; `.cline/skills` dropped                | `.clinerules/*.md` for non-always-on items and glob context, `.cline/agents/<id>.yaml`                                             |
| `junie`                        | natively                             | shared `AGENTS.md`                          | shared; `.junie/skills` dropped                | `.junie/rules/*.md` for non-always-on items, `.junie/agents/<id>.md`                                                             |

Every preset above still writes its MCP file where it did before. Declarative providers (`amp`, `hermes`, `claude`,
`junie`, ...) honor the flag as well. Custom presets and provider specs that are not in the table take no part, and
their `AGENTS.md` or `.agents/skills` output is dropped in favor of the shared copy (see
[Overlapping writers](#overlapping-writers)).

### How each tool finds AGENTS.md

- **Native readers** (`codex`, `opencode`, `xum`, `amp`, `pi`, `hermes`, `cursor`, `copilot`, `devin`, `cline`,
  `junie`, `antigravity`): nothing to configure.
- **Claude Code** reads `CLAUDE.md`, and only reads `AGENTS.md` itself from v2.1.277 and in every session from
  v2.1.281. ai-rulez therefore keeps `CLAUDE.md` as a generated shim: the generated-file banner followed by
  `@AGENTS.md`. The import works on every version and never loads the file twice.
- **Gemini CLI** loads only the names in `context.fileName`. With the flag on, ai-rulez merges `"AGENTS.md"` (and
  `"GEMINI.local.md"`) into `context.fileName` in `.gemini/settings.json`, keeping existing names and other keys (a
  single-string value becomes a list), and writes the file even when there are no `[[mcp_servers]]`. Gemini replaces its default
  `GEMINI.md` with whatever is configured, which is why `GEMINI.md` is not written.

### Why copilot-instructions.md, .hermes.md and friends are dropped

Some tools load a single instruction file and pick it by order, so a preset's own root file would hide `AGENTS.md`:

- **Zed** takes the first existing of `.rules`, `.cursorrules`, `.devin/rules`, `.clinerules`,
  `.github/copilot-instructions.md`, `AGENT.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`. A generated
  `.github/copilot-instructions.md` would hide `AGENTS.md` from Zed, so the `copilot` preset does not write it.
  `CLAUDE.md` and `GEMINI.md` come later and do not shadow it. `.clinerules` is a directory here, which Zed does not
  treat as a rules file.
- **Hermes** loads one context type, and `.hermes.md` shadows the whole `AGENTS.md` chain; the `hermes` preset does
  not write it.
- **Warp** prefers `WARP.md` in the same directory. No preset writes it; do not add one by hand.
- **Junie** prefers `.junie/AGENTS.md`; otherwise it reads the project-root `AGENTS.md` together with `.junie/rules`
  and `.junie/playbook.md`. The legacy `.junie/guidelines.md` layout is no longer written; the root `AGENTS.md` is the
  open-standard file Junie reads.

### Overlapping writers

If another preset (including a custom provider) also writes `AGENTS.md` or a file under `.agents/skills`, the shared
output wins and the other copy is not written. Without this, the last preset in name order would overwrite the
shared file.

## Where rules and context go

The shared `AGENTS.md` always carries always-on rules and context, and anything scoped only by negated globs
(`!gen/**`), which no rules folder can express. What else it carries depends on the presets that rely on it.

| Kind                                      | Rules folder presets (`cursor`, `devin`, `cline`, `claude` and `antigravity` in split mode, `junie` in split mode) | Presets without a folder (`codex`, `opencode`, `amp`, `xum`, `pi`, `hermes`, `gemini`) |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| Always-on rule or context                 | `AGENTS.md` only                                                                                                                      | `AGENTS.md`                                                                      |
| Glob-scoped rule or context               | the preset's folder                                                                                                                   | `AGENTS.md` with an `_Applies to_` line                                          |
| Auto rule                                 | the folder                                                                                                                            | `AGENTS.md` with a `_When relevant_` line                                        |
| Manual rule                               | the folder                                                                                                                            | `AGENTS.md`                                                                      |
| Only negated globs (`!gen/**`)            | `AGENTS.md` only                                                                                                                      | `AGENTS.md`                                                                      |

The decision is made once for the whole file, from the configured built-in presets that rely on it:

| Configured preset (rules mode)                         | Scoped items (glob rules, glob context) in `AGENTS.md` | Auto and manual items in `AGENTS.md` |
| ------------------------------------------------------ | ------------------------------------------------------ | ------------------------------------ |
| `codex`, `opencode`, `amp`, `xum`, `pi`, `hermes`, `gemini`  | yes                                                    | yes                                  |
| `claude`, `antigravity` (split)                        | no                                                     | no                                   |
| `claude`, `antigravity` (inline)                       | no                                                     | yes                                  |
| `cursor`, `devin`, `cline` (either)                    | no                                                     | no                                   |
| `copilot` (either)                                     | no                                                     | yes, its folder only holds `applyTo` items |
| `junie` (split)                                        | no                                                     | no                                   |
| `junie` (inline)                                       | yes, it writes no rule files                           | yes                                  |

The most inclusive row among the configured presets wins. In `inline` mode `claude` and `antigravity` still write
glob-scoped files, so only auto and manual items move into `AGENTS.md`. `cursor` ignores the mode: it always writes
files. Scoped context is inlined together with scoped rules.

### Duplication trade-off

The inlining is decided for the file, not per reader. When a preset without a folder is enabled next to a preset
with one, the scoped items appear in `AGENTS.md` and in the folder, so the tool with the folder loads them twice.
Examples: `claude` + `codex`, `cursor` + `codex`, `cursor` + `gemini`, `cursor` + `amp`, `cursor` + `hermes`, and
(auto and manual items only) `claude` in `inline` mode + `cursor`. The research found no cross-file deduplication for
Cursor or Claude Code, and none for the Copilot cloud agent. A mixed setup that wants no duplication should keep to
presets that all have folders, accept the overlap, or leave `agents_md` off.

Machine-local rules are never part of the shared `AGENTS.md`. They keep their per-preset files (for example
`.github/instructions/<id>.local.instructions.md`, `CLAUDE.local.md`, `AGENTS.override.md`). See
[Local Configuration](local-overrides.md#generated-output) for the file each tool loads.

## Skills

- Skills without `targets` are written once to `.agents/skills/<name>/SKILL.md`, with bundled resources, in the
  generic Agent Skills format (`name`, `description`). Codex's `metadata.short-description` is not emitted there.
- **Claude Code** reads `.claude/skills`, not `.agents/skills`, so the `claude` preset keeps writing its own copy.
- Skills that set `targets` stay out of the shared tree: they are written to the per-preset skills directories of the
  presets the targets allow, exactly as without the flag. A skill targeting `codex` goes to Codex's own skills directory (`.agents/skills`, or `codex_skills_dir`), the only
  place Codex reads, so other `.agents/skills` readers see it too; an untargeted skill goes to the shared tree; a skill targeting a preset that is not configured is written nowhere.
- `.agents/skills` is read by Codex, Cursor, Copilot, Devin, Gemini CLI, Antigravity, Junie, Cline, opencode, Amp,
  Hermes, pi, Zed, Warp and others, per the table above.

## targets

Frontmatter `targets` select the presets or files an item is written for. The shared file changes who an item can
reach:

- A rule or context item is included in `AGENTS.md` when its `targets` name a preset that relies on the file
  (`claude`, `gemini`, `cursor`, `codex`, ...), the root file such a preset replaced (`CLAUDE.md`, `GEMINI.md`,
  `.hermes.md`, `.github/copilot-instructions.md`, by path or base name), or any default
  owner of `AGENTS.md` (`codex`, `opencode`, `xum`, `amp`, `pi`, `junie`), configured or not.
- A target naming an unconfigured preset's root file (for example `GEMINI.md` with only `codex` configured) does not
  select the item for `AGENTS.md`.
- **Widening:** the file is shared, so an always-on rule targeted at a single preset now reaches every tool that reads
  `AGENTS.md`.
- **Folder-only targets:** an always-on rule or context item whose `targets` match only a rules folder or rule file
  path (`.cursor/rules/`, `.claude/rules/`, `.github/instructions/`, `.devin/rules/`, ...) and no `AGENTS.md`
  owner or root file is not in `AGENTS.md`. The preset writes it as a rule file in that folder, exactly as without
  the flag. To limit an always-on item to one tool, target its rules folder path rather than its preset name.
- Skills with `targets` are covered under [Skills](#skills).

## Scopes, local files and hashes

**Scopes.** Each `[[scopes]]` entry gets a nested `<scope>/AGENTS.md` carrying only the scope's own domain content.
A scope's `claude` preset writes `<scope>/CLAUDE.md` as a shim with `@AGENTS.md`, so Claude Code imports the nested
file. Gemini CLI reads project settings only, so `.gemini/settings.json` is written at the root and not in scopes;
the root setting is what makes Gemini find nested `AGENTS.md` files. If `gemini` is configured for a scope but not for
the root, Gemini gets no instructions for that scope and `generate` warns; add `gemini` to the root presets.

**Local overrides.** Machine-local content is not part of `AGENTS.md`.

- `CLAUDE.local.md` is still written when local content exists. Claude Code ignores `AGENTS.md` when a
  `CLAUDE.local.md` exists, but `CLAUDE.md` imports it explicitly, so nothing is lost.
- Tools that read an AGENTS chain get local content through the file they load for it. Codex, and Hermes with the
  flag on, load `AGENTS.override.md` instead of `AGENTS.md`, so ai-rulez writes a git-ignored `AGENTS.override.md`
  that repeats the shared `AGENTS.md` and appends the local rules and context. OpenCode lists `AGENTS.local.md` in
  `opencode.json` `instructions`; xum appends `AGENTS.local.md` itself. Amp and pi have no project-local file, so
  their local content is not written and `generate` warns. Claude Code is covered through `CLAUDE.local.md`.
- Gemini CLI loads `GEMINI.local.md` because `.gemini/settings.json` `context.fileName` lists it: ai-rulez writes
  `["AGENTS.md", "GEMINI.local.md"]` (`["GEMINI.md", "GEMINI.local.md"]` with the flag off), whether or not local
  content exists. A `context.fileName` you wrote yourself is kept and `GEMINI.local.md` is appended to it; `clean`
  takes back only the names ai-rulez added.
- Junie and Antigravity load the local context from their rules folders (`.junie/rules/ai-rulez.local.md`,
  `.agents/rules/ai-rulez.local.md`).

**Hashes.** The shared outputs carry one `Source-Hash` (and `Content-Hash`, per `[header] hashes`) computed from the
content and the settings that shape the file. It does not depend on the list of presets, rules modes, MCP servers or
plugins, so adding a preset that reads the file leaves the provenance line unchanged unless the file itself changes.
It changes with the inlining decision above, and, only when some rule or context item has `targets`, with the set of
presets relying on the file. `AGENTS.md` and the files under `.agents/skills` share the hash. The `[header] hashes`
modes apply as elsewhere: `full` writes both hashes, `content` only `Content-Hash`, `none` neither.

## Turning the flag on and off

```bash
ai-rulez generate        # after editing agents_md in .ai-rulez/config.toml
```

- **On:** files that were only needed by the per-tool layout (`GEMINI.md`, `.hermes.md`,
  `.github/copilot-instructions.md`, `.devin/skills`, ...) are removed
  through the generated manifest. `CLAUDE.md` is rewritten as the shim.
- **Off:** the per-tool files are regenerated and the shared `AGENTS.md` and `.agents/skills` files that no preset
  writes itself are removed. An off, on, off sequence ends where it began, except for the Gemini setting below.
- **`.gemini/settings.json`:** on toggle-off, a `context.fileName` that exactly equals a value ai-rulez wrote
  (`["AGENTS.md"]`, `["AGENTS.md", "GEMINI.local.md"]`) is rewritten to `["GEMINI.md", "GEMINI.local.md"]`, whether or
  not ai-rulez wrote the whole file. In a list you authored, only the `AGENTS.md` ai-rulez appended is removed; if the
  value lists `AGENTS.md` but not `GEMINI.md`, `generate` warns so you can add it yourself. Other keys and existing
  `mcpServers` are untouched.
- **Hand-written files** are never removed. A skill you wrote at `.codex/skills/mine/SKILL.md` or
  `.agents/skills/mine/SKILL.md` survives any number of toggles, because only manifest-tracked files are cleaned up.

## Caveats

- **Claude Code before v2.1.277** does not read `AGENTS.md` by itself; the `@AGENTS.md` shim covers it. Claude Code
  also drops `AGENTS.md` whenever any `CLAUDE.md` is found, which is why the shim imports rather than relies on
  discovery.
- **Duplication is possible** in mixed setups (see the [trade-off](#duplication-trade-off)). The Copilot cloud agent
  and Copilot in VS Code provide all relevant instruction sets and document no deduplication; Copilot CLI only removes
  identical copies.
- **Size limits** apply to the one larger file: Codex reads at most 32 KiB of project instructions by default
  (`project_doc_max_bytes`), Kimi CLI caps at 32 KiB, and Antigravity reads 24 KB per file and 20k tokens in
  aggregate. Use `compact = true` or move bulk content to skills when the file grows.
- **Cursor** documents neither co-presence nor deduplication of `AGENTS.md` with `.cursor/rules`; the preset keeps
  only non-always-on items in `.cursor/rules` so the same text is not in both.
- **Nested `AGENTS.md`** support varies: Codex reads only from the project root down to cwd, Cursor, Copilot, Amp
  and opencode load nested files; Zed does not; Cline and Junie are unverified.
- **Not verified:** the entries marked `?` in the tool table, and whether Copilot expands the `@AGENTS.md` line of
  the Claude shim when it reads `CLAUDE.md` as agent instructions. Status on 2026-10-04: a `?` means no
  dated research pass has confirmed the behavior. The 2026-10-04 pass re-checked only the Codex `project_doc_max_bytes`
  default (32 KiB, content past it is dropped), skill `paths` in Cursor, and Copilot `excludeAgent`; the other `?` cells
  were not re-researched. In particular `.codex/agents`, `.codex/commands`, `.github/agents` and `.github/commands`,
  which `generate` writes, remain unverified as outputs the tools read.

See also: [Configuration: `agents_md`](configuration.md#agents_md), [Rules and native rules folders](rules.md),
[Local Overrides](local-overrides.md), [Monorepo](monorepo.md).
