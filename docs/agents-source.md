# .agents as a source

With `source = "agents"`, ai-rulez reads your content from the open standards (the root `AGENTS.md` and the
`.agents/` directory) instead of `.ai-rulez/`, and generates every other tool's files from them. Use it when your team
already keeps its instructions and skills there and wants Claude Code, Cursor, Gemini, Copilot and the rest to get
the same content without moving a file. ai-rulez never writes `AGENTS.md` or anything under `.agents/` in this mode.

## Example

```bash
ai-rulez init --source agents
```

This writes `.agents/ai-rulez.toml` and, if the project has none, a starter `AGENTS.md`. Choose the presets and add
content:

```toml
# .agents/ai-rulez.toml
version = "5.0"
name = "acme"
presets = ["claude", "codex", "cursor", "gemini"]
```

```text
AGENTS.md                          # project instructions
svc/api/AGENTS.md                  # instructions for svc/api
.agents/skills/release/SKILL.md    # an Agent Skill
.agents/rules/go-style.md          # a rule with paths: ["**/*.go"]
```

```text
$ ai-rulez generate
INFO  source = agents: AGENTS.md and .agents/ are the source and are not written skipped=12
INFO  Generation complete files=11
```

The result:

```text
CLAUDE.md                              # AGENTS.md text, inline
GEMINI.md                              # AGENTS.md text and rules
.claude/rules/go-style.md
.claude/rules/context-agents-svc-api.md   # svc/api/AGENTS.md, scoped to svc/api/**
.claude/skills/release/SKILL.md        # Claude Code does not read .agents/skills
.cursor/rules/go-style.mdc
.gemini/settings.json
...
```

Codex and Cursor read `AGENTS.md` and `.agents/skills` themselves, so they get no copy of the `AGENTS.md` text or the
skills; Cursor still gets `.cursor/rules/go-style.mdc`.

## Layout

ai-rulez reads what the standards define as they define it, and uses folders named after their content for the rest.

| Content | Location | Defined by |
| --- | --- | --- |
| Project instructions | root `AGENTS.md` | [AGENTS.md](https://agents.md) |
| Scoped instructions | `<dir>/AGENTS.md` | AGENTS.md (nested files) |
| Skills, with `references/`, `scripts/`, `assets/` | `.agents/skills/<name>/SKILL.md` | [Agent Skills](https://agentskills.io) |
| Rules | `.agents/rules/<name>.md` | Antigravity convention |
| Agents (subagents) | `.agents/agents/<name>.md` | Antigravity and Amp convention |
| Review checks | `.agents/checks/<name>.md` | Amp convention |
| Commands | `.agents/commands/<name>.md` | ai-rulez |
| Context files | `.agents/context/<name>.md` | ai-rulez |
| Domains | `.agents/domains/<name>/{rules,context,skills,agents,commands,checks}` | ai-rulez |

Each file has the format described in [Configuration](configuration.md#content-layout). Hooks, MCP servers,
permissions, presets, profiles and roles stay in the TOML configuration.

## Where the configuration lives

| Configuration file | `source` | ai-rulez state (manifest, `ai-rulez.lock`, `config.local.toml`, layers, `local/`) |
| --- | --- | --- |
| `.agents/ai-rulez.toml` (preferred) | implied `"agents"` | `.agents/.ai-rulez/` |
| `.ai-rulez/config.toml` | set `source = "agents"` | `.ai-rulez/` (its content folders are not read) |

When both files exist, `.agents/ai-rulez.toml` is used and `doctor` warns about the shadowed one. `config.<name>.toml`
layers and `config.local.toml` are read from the state directory, never from beside `.agents/ai-rulez.toml`.

## What is read

- **Root `AGENTS.md`**: one context item named `agents`. Its first `# Title` is dropped when rendered. `list context`
  shows it with `AGENTS.md` as its path; `edit` and `remove` refuse it unless you split the file (below).
- **Nested `AGENTS.md`**: context scoped to its directory (see [below](#nested-agentsmd)).
- **`.agents/`**: scanned like the `.ai-rulez` tree, with the same formats, `bundle_exclude` and symlink policy (a
  symlink must resolve inside the project). A symlinked `AGENTS.md` is never read.
- **Generated files are ignored**: a file whose header is the ai-rulez generated banner is output of an earlier run,
  not source. `generate` names such files in a warning; delete them or replace them with your own.

## Splitting AGENTS.md

Set `agents_md_split = true` to read the root `AGENTS.md` as one context item per `##` heading, so each section can
be listed, shown, edited and removed on its own:

```toml
agents_md_split = true
```

```text
$ ai-rulez list context
Context:
  (root):
    • agents
    • agents-conventions
    • agents-code-style
```

- The text before the first `##` is `agents`. Each section is `agents-<heading slug>` (`## Code Style` is
  `agents-code-style`); a repeated heading gets `-2`, `-3`. A heading inside a code fence does not split.
- `show context <item>` (MCP `read_context`) prints the section, heading line included. `edit context <item>` (MCP
  `update_context`) replaces it; `remove context <item>` (MCP `delete_context`) cuts it out. Every other byte of
  `AGENTS.md`, its line endings, BOM and file mode are kept.
- An edit that adds or drops a `##` heading, renames the section or is empty is refused.
- Items are rendered in name order, not in file order.
- Every item pins the whole file in `ai-rulez.lock`, so editing one section marks all of them changed for
  `lock --check`. Run `ai-rulez lock` to accept.

The setting requires the agents source; `validate` rejects it otherwise.

## Nested AGENTS.md

`svc/api/AGENTS.md` becomes the context item `agents-svc-api`, scoped to `svc/api/**`, and each tool renders it the
way it renders a path-scoped rule (for example `.claude/rules/context-agents-svc-api.md` with `paths: [svc/api/**]`).
Tools that read nested `AGENTS.md` files themselves get no copy.

The search:

- skips symlinks, hidden directories and dependency and build trees (`node_modules`, `vendor`, `target`, `dist`,
  `build`, ...);
- honours `.gitignore` inside a git repository;
- stops at a directory with its own `.agents/ai-rulez.toml`, `.ai-rulez/config.toml` or
  `.config/ai-rulez/config.toml`, which is a separate project (`generate --recursive` handles it);
- goes at most eight directories deep and warns when it stops early;
- leaves out generated files, empty files and files that only point at another (`@AGENTS.md`).

When two directories produce the same slug (`a-b/` and `a b/`), each gets a short hash of its path
(`agents-a-b-1f2e3d4c`). Adding or removing one of them renames the other too.

Nested files are not split, and `list`, `show`, `edit` and `remove` do not see them: edit them by hand.

## What generate writes

`generate` writes every tool's files except those that would land on `AGENTS.md`, a nested `AGENTS.md` or anything
under `.agents/`. Those outputs are dropped with one info line, so your files stay byte for byte and
`generate --check` stays clean.

- Every tool gets full files, as with `agents_md = false`: `CLAUDE.md`, `GEMINI.md` and the rules folders carry
  your always-on rules and context; skills, agents and commands go to each tool's own folders. `agents_md = true` is
  rejected.
- The `agents` item (your `AGENTS.md` text) is left out of the files of tools that load `AGENTS.md` themselves
  (Codex, Cursor, Copilot and the others in the [tool table](agents-md.md#tool-support)), so they never see it twice.
  Gemini CLI gets it in `GEMINI.md`.
- Such a tool receives your rules, agents and commands only where it has a place for them. Codex has no rules folder,
  so it reads only `AGENTS.md` and `.agents/skills`. Put what every tool must follow in `AGENTS.md`.
- An output below a symlink that leads into `.agents` (`.claude/skills -> ../.agents/skills`) is dropped with a
  warning, so nothing is written through the link.
- `generate --plugin` skips plugin outputs under `.agents/` (such as the Codex index
  `.agents/plugins/marketplace.json`) with a warning.

## Commands

| Command | Behavior with the agents source |
| --- | --- |
| `init --source agents` | Writes `.agents/ai-rulez.toml` and a starter `AGENTS.md`. `--config-dir .ai-rulez` writes `.ai-rulez/config.toml` with `source = "agents"` instead; `--force` keeps the old file as `<file>.bak-<timestamp>` |
| `init --source agents --from CLAUDE.md,.cursor`, `convert --into .agents` | Import other tools' files into the layout above. Files already in place are left as they are |
| `add`, `remove`, `list`, `show`, `edit` | Work on `.agents/rules`, `.agents/skills`, `.agents/domains/<name>` and the rest; machine-local content goes to `.agents/.ai-rulez/local` |
| `clean` | Never removes `AGENTS.md` or anything under `.agents`, even with `--include-edited` |
| `lock` | Pins `AGENTS.md`, nested `AGENTS.md` files and the `.agents` tree |
| `generate --watch` | Watches `AGENTS.md`, `.agents` and the nested `AGENTS.md` files found at start or after each run; a new nested file is picked up after the next run |
| `doctor` | Warns about a shadowed `.ai-rulez/config.toml`, `.ai-rulez` content that is not read, and generated files left in the source |
| `generate --recursive`, `validate --recursive` | Find nested `.agents/ai-rulez.toml` projects |
| `migrate okf` | Refused: it converts the `.ai-rulez` tree |

The [`[guard]`](settings.md#guard) hook keeps agents out of generated files only; `AGENTS.md` and `.agents` stay
editable.

To return to the `.ai-rulez` tree, set `source = "ai-rulez"` in `.ai-rulez/config.toml` and move the files back, or
run `ai-rulez convert --from native` over the generated tool files.
