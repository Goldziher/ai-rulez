# Baz

The `baz` preset writes what the [Baz](https://baz.ai) pull request reviewer reads from a repository. Baz has no
repository-level config file: its settings live in its web UI, and the repository influences it only through
instruction files. This page records what Baz documents, what the preset generates from it, and what is not documented.

```toml
presets = ["claude", "baz"]

[rules]
baz_scoped = "nested"   # nested (default) | root
baz_default = "all"     # all (default) | none
# baz.include = ["db-dao"]  # with baz_default = "none", opt these rules in ([rules.baz] table)
baz_rest = "root"        # root (default) | skip: path-scoped rules baz does not select (baz_default = "none", !baz)
baz_unscoped = "root"   # root (default) | skip: path-scoped rules with no directory
baz_dedupe = false      # write a shared parent AGENTS.md when globs share one
baz_dedupe_max = 0      # collapse a rule with more than K nested copies to a covering directory (0 = off)
baz_budget = 0          # byte budget of the Baz AGENTS.md surface (0 = off)
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
- a directory does not exist in the project, is a `[[scopes]]` path, is inside the configuration directory, `.git` or
  another ai-rulez root nested in the project (that root owns its directory; `generate` warns and names the rule), or
  escapes the project root;
- `baz_scoped = "root"`.

The root `AGENTS.md` lacks the items that moved, in every preset that writes it, so `codex`, the shared `agents_md` file
and `baz` stay identical. The scope of a nested file is the whole directory, which can be wider than the glob (the
`_Applies to:_` line keeps the exact globs for the reader).

### Path-scoped rules with no directory

A rule scoped by `**/BUILD`, `**/*.go` or `.mcp.json` names no directory, so it cannot be nested and the root
`AGENTS.md` inlines it with its `_Applies to:_` line. With `presets = ["claude", "baz"]` Claude imports that file in
every session and also loads the same rule from `.claude/rules/`, so the rule is paid for twice. `baz_unscoped = "skip"`
leaves such rules out of the root file. It applies only to the contribution `baz` makes: the root `AGENTS.md` is shared
with `codex`, `opencode`, `amp`, `xum` and the other `AGENTS.md` presets, which need the rule, so while one of them is
configured the file keeps it and `skip` has no effect. It also has no effect with `baz_scoped = "root"`, which keeps
every path-scoped item in the root file by design. The default `"root"` leaves the output unchanged.

### Rules baz does not select

With `baz_default = "none"` the rules that do not opt in, and rules excluded by `!baz`, are not nested, so the root
`AGENTS.md` keeps inlining them. When baz is meant as an opt-in surface, `baz_rest = "skip"` leaves every such
path-scoped rule out of the root file. Like `baz_unscoped`, it applies only when `baz` is the only preset writing the
root `AGENTS.md` and `baz_scoped = "nested"`. The default `"root"` leaves the output unchanged.

When `baz_budget` is exceeded the warning also names the path-scoped rules the root file inlines for this reason.

## Narrowing, deduping and budgeting the nested surface

One rule with many globs is copied into every directory those globs point into, so a large monorepo can grow a nested
`AGENTS.md` per service. Four `[rules]` keys keep that surface under control; all default to today's behaviour.

- **`baz_default = "none"`** nests only items whose frontmatter `targets` select `baz`; an item with no targets is not
  nested. It stays in the root `AGENTS.md`, which is shared with `codex`, `opencode`, `amp` and `xum` and is read by
  Baz, so nothing is dropped — it is simply not duplicated into the directory files. The default `"all"` nests every
  path-scoped item.
- **`baz_dedupe = true`** writes an item whose globs point into several sibling directories under one parent once, to
  that parent's `AGENTS.md` (for example `services/AGENTS.md` instead of `services/api/AGENTS.md` and
  `services/web/AGENTS.md`). It is off by default, so the output is unchanged.
- **`baz_dedupe_max = <K>`** collapses an item that would be copied into more than K directories to the directory
  covering all of them, even without `baz_dedupe`. When the directories share nothing but the repository root (a rule
  scoped to `platform/`, `deploy/` and `dev_tools/`), the item goes to the root `AGENTS.md`, the baz contribution to that
  shared file. With `baz_unscoped = "skip"` the root is off limits for path-scoped items, so the copies stay. `0`
  disables it.
- **`baz_budget = <bytes>`** makes `generate` warn when the total size of the Baz `AGENTS.md` surface (the root file
  plus the nested ones) exceeds the budget, naming the largest files and the budget. It is advisory and never fails the
  run. `0` disables it.

The Claude surface is identical in every mode: only the Baz nested `AGENTS.md` files change.

### Per-rule opt-in and opt-out

Instead of tagging every other rule with `targets`, pick the rules Baz should get:

- **`targets: ["!baz"]`** excludes one rule from everything the `baz` preset writes of its own (the nested
  `AGENTS.md` files, skills and agents). A negated target wins over any positive one, and a rule whose targets are all
  negated is still written everywhere else. The root `AGENTS.md` is shared with `codex`, `opencode`, `amp` and `xum`
  and must stay identical, so `!baz` does not remove a rule from it; use a target that names the other presets to
  limit it.
- **`[rules.baz] include = ["db-dao", ...]`** or **`baz: true`** in a rule's frontmatter opts a rule in when
  `baz_default = "none"`; it makes no difference under the default `"all"`, which already nests every item. `include`
  matches rule and context names (case-insensitive, with or without `.md`).

The older workaround still works: a directory target such as `targets: [".claude/rules/"]` selects only the outputs
under that folder, so the rule is not written to the Baz `AGENTS.md` files. `!baz` and `include` say the same thing
without naming a folder.

## Claude Code and the nested files

Per Anthropic's [memory documentation](https://code.claude.com/docs/en/memory#agents-md) (read 2026-10-12), Claude Code
reads `AGENTS.md` natively (v2.1.277 or later) **only when there is no `CLAUDE.md`, `.claude/CLAUDE.md` or
`CLAUDE.local.md` in the working directory or any directory above it**. With any of those present it reads the
`CLAUDE.md` files only (the default **Project instructions** value, `claude-md-or-agents-md`). What that means here:

- **With the `claude` preset's root `CLAUDE.md` shim** (the default, `claude_md = "shim"`): a nested, baz-only
  `AGENTS.md` is not loaded by Claude Code, because a `CLAUDE.md` exists above every directory. Only the shim's
  `@AGENTS.md` import reaches Claude, and that pulls in the root file alone. A scoped rule therefore reaches Claude from
  `.claude/rules/` only, not twice, and `targets` is not needed to avoid a double load. The root `AGENTS.md` is read by
  Claude through the import, so rules the root file inlines (see `baz_unscoped`) are loaded in addition to their
  `.claude/rules/` copy.
- **Without a `CLAUDE.md` anywhere** (`claude_md = "none"`, or no `claude` preset): Claude reads the root `AGENTS.md` at
  session start and, per the documentation, a subdirectory's `AGENTS.md` when it opens a file there with the Read
  tool. A scoped rule can then reach Claude both from `.claude/rules/` (if the `claude` preset also writes it) and from
  the nested file. Limit it with `targets` (`!baz` on the rule, or a target naming only the outputs you want).
- **With `Project instructions` set to `claude-md-and-agents-md`** Claude reads both file kinds, so the shim case behaves
  like the previous bullet.

Not settled by the documentation: whether the on-demand subdirectory load is also suppressed when only a root
`CLAUDE.md` exists (the table says "your `CLAUDE.md` files only", but the subdirectory bullet restates only the
subdirectory's own `CLAUDE.md` files as the condition). This has not been probed against a running Claude Code, so
treat the shim case above as the documented default rather than a verified one.

## Using it with other presets

- **`claude`**: Baz already reads `.claude/skills/` and `.claude/agents/`, so `baz` writes neither skills nor agents
  and nothing is read twice. `CLAUDE.md`, `.claude/rules/` and the rest of the `claude` output are unchanged. Two
  consequences: `claude` renders commands as user-invocable skills under `.claude/skills/`, which Baz reads as
  skills (its "ignore commands" rule names only the `commands/` directories); and whether Claude Code also loads the
  nested `AGENTS.md` files depends on `CLAUDE.md`, see [Claude Code and the nested files](#claude-code-and-the-nested-files).
- **`codex`, `opencode`, `amp`, `xum`**: they share `AGENTS.md`, which is identical. With `agents_md` on (the default) the shared
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
