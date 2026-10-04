# Baz

The `baz` preset writes what the [Baz](https://baz.ai) pull request reviewer reads from a repository. Baz has no
repository-level config file: its settings live in its web UI, and the repository influences it only through
instruction files. This page records what Baz documents, what the preset generates from it, and what is not documented.

```toml
presets = ["claude", "baz"]

[rules]
baz_scoped = "nested"   # nested (default) | root
```

## What Baz reads

Verified against [Skills & Instructions](https://baz.ai/docs/agents/skills-and-instructions) on 2026-10-04. Only the
documented patterns below are candidates; anything else is not an instruction file.

| Pattern                                                                    | Depth                     |
| -------------------------------------------------------------------------- | ------------------------- |
| `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, `.clauderc`, `.coderabbit.yaml`, `*BUGBOT.md` | any depth, scoped to the file's directory and below |
| `.cursor/rules/*.md(c)`, `.cursor/BUGBOT.md`, `.claude/*.md`, `.claude/agents/*.md(c)`, `.agent(s)/*.md(c)` | repository root only |
| Skill folders `.claude/skills/*`, `.cursor/skills/*`, `.agent(s)/skills/*` (need a top-level `SKILL.md` or `AGENTS.md`) | repository root only |

Always ignored: `.claude/commands/`, `.cursor/commands/`. `.claude/rules/` appears in no documented pattern, so rules
written there are not read. There is no precedence between guidelines: every one whose directory covers a changed file
applies. Files are read from the **default branch** (a full scan on connect, then an incremental scan per push to it);
edits on a feature branch are not used to review that same branch. Baz keeps only rules a reviewer can check by reading
code and drops agent-behaviour, process and "run this tool" instructions, so write review-relevant rules as code
conventions.

Not documented, and therefore not encoded: size limits, and whether draft pull requests are reviewed (the settings page
lists "Review draft PRs" as an off-by-default per-repository UI setting).

## What the preset generates

| Output                                   | Content                                                                                   |
| ---------------------------------------- | ----------------------------------------------------------------------------------------- |
| `AGENTS.md`                              | Root rules and context, byte-identical to the file `codex`, `opencode`, `amp` and `xum` write |
| `<dir>/AGENTS.md`                        | Path-scoped rules and context whose globs point into `<dir>` (see below)                  |
| `.agents/skills/<id>/SKILL.md` + resources | Skills, in the generic Agent Skills format                                                |
| `.claude/agents/<id>.md`                 | Agents, with `name` and `description` frontmatter                                         |

It writes no commands and no `.claude/rules/`. Skills and agents are root-only in Baz, so a `[[scopes]]` run writes only
its scoped `AGENTS.md`. An item with `targets` is written only when a target names `baz` or `AGENTS.md`; `baz` is a
default owner of `AGENTS.md` like `codex`, so a target naming any of them selects the item for the shared file.

## Path-scoped rules

Baz cannot see `.claude/rules/`, and a root `AGENTS.md` applies to every change. With `baz_scoped = "nested"` a rule or
context item with `paths`/`globs` is written to the `AGENTS.md` of the directory its globs point into, which Baz scopes
to that directory:

```markdown
---
paths:
  - services/api/**/*.py
---
```

becomes `services/api/AGENTS.md` with the rule under `## Rules` and its `_Applies to:_` line. The directory is the leading
wildcard-free path segments of each positive glob (`{a,b}` is expanded; several directories get a copy each; a directory
below another listed one is covered by it). The item stays in the root `AGENTS.md`, exactly as the other `AGENTS.md`
presets write it, when:

- any positive glob has no directory (`**/*.py`, `*.md`), or only negated globs are set;
- a directory does not exist in the project, is a `[[scopes]]` path, is inside the configuration directory, `.git`, or
  escapes the project root;
- `baz_scoped = "root"`.

The root `AGENTS.md` lacks the items that moved, in every preset that writes it, so `codex`, the shared `agents_md` file
and `baz` stay identical. The scope of a nested file is the whole directory, which can be wider than the glob (the
`_Applies to:_` line keeps the exact globs for the reader).

## Using it with other presets

- **`claude`**: Baz already reads `.claude/skills/` and `.claude/agents/`, so `baz` writes neither skills nor agents
  and nothing is read twice. `CLAUDE.md`, `.claude/rules/` and the rest of the `claude` output are unchanged. Two
  consequences: `claude` renders commands as user-invocable skills under `.claude/skills/`, which Baz reads as
  skills (its "ignore commands" rule names only the `commands/` directories); and Claude Code loads a nested `AGENTS.md`
  when it works in a directory without a `CLAUDE.md`, so a scoped rule can reach Claude both from `.claude/rules/` and
  from the nested file. Set `targets` on the rule to limit it to one of them.
- **`codex`, `opencode`, `amp`, `xum`**: they share `AGENTS.md`, which is identical. With `agents_md = true` the shared
  `AGENTS.md` and `.agents/skills/` are written once and `baz` adds only its nested files, agents and targeted skills.
- **`cursor`** and others that write `.cursor/rules`: Baz reads those files too, so a rule can be read from both
  `.cursor/rules` and `AGENTS.md`. Baz applies all in-scope guidelines, so contradictions show up as contradictory
  feedback.

## Limits

- Nothing reaches Baz until it is on the default branch.
- `baz` cannot set anything in the Baz UI (review drafts, ignored branches, custom reviewers). Those are configured by an
  organization admin; the read-only `GET /api/v2/reviewer/config` and `/reviewers` API can audit them.
- Nested `.claude/` and `.cursor/` directories are not read by Baz (root only). The preset never writes them, but
  another preset in a `[[scopes]]` run does.
- The `baz` preset has no machine-local variant: Baz reads committed files only.
