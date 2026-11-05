# Baz

The `baz` preset writes the files the [Baz](https://baz.ai) pull request reviewer reads from a repository. Baz has no
repository config file: its settings live in its web UI, and a repository influences it only through instruction
files. The preset turns path-scoped rules into `AGENTS.md` files in the directories they apply to, so Baz applies each
rule only to the code it is about.

```toml
# .ai-rulez/config.toml
presets = ["baz"]
```

```markdown
<!-- .ai-rulez/rules/api.md -->
---
type: Decision
title: API
x-ai-rulez:
  kind: rule
  id: api
  metadata:
    paths:
      - "services/api/**/*.py"
---

Validate request bodies.
```

`ai-rulez generate` writes the root `AGENTS.md` with the always-on rules, and the path-scoped rule into the directory
its glob points to:

```markdown
<!-- services/api/AGENTS.md (generated header omitted) -->
# services/api

Instructions for `services/api/` and everything below it.

## Rules

### api

_Applies to: `services/api/**/*.py`_

Validate request bodies.
```

## What Baz reads

From Baz's [Skills & Instructions](https://baz.ai/docs/agents/skills-and-instructions):

| Pattern | Where |
| ------- | ----- |
| `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, `.clauderc`, `.coderabbit.yaml`, `*BUGBOT.md` | any depth, scoped to the file's directory and below |
| `.cursor/rules/*.md(c)`, `.cursor/BUGBOT.md`, `.claude/*.md`, `.claude/agents/*.md(c)`, `.agent(s)/*.md(c)` | repository root only |
| Skill folders `.claude/skills/*`, `.cursor/skills/*`, `.agent(s)/skills/*`, each with a top-level `SKILL.md` or `AGENTS.md` | repository root only |

- `.claude/commands/` and `.cursor/commands/` are ignored, and `.claude/rules/` is not read.
- There is no precedence: every guideline whose directory covers a changed file applies.
- Files are read from the **default branch**. An edit on a feature branch does not affect the review of that branch.
- Baz keeps rules a reviewer can check by reading code and drops agent-behavior, process and "run this tool"
  instructions, so write review rules as code conventions.

## What the preset generates

| Output | Content |
| ------ | ------- |
| `AGENTS.md` | Always-on rules and context; the same file `codex`, `opencode`, `amp` and the other `AGENTS.md` presets write |
| `<dir>/AGENTS.md` | Path-scoped rules and context whose globs point into `<dir>` |
| `.agents/skills/<id>/SKILL.md` and resources | Skills |
| `.claude/agents/<id>.md` | Agents, with `name` and `description` frontmatter |

It writes no commands, no MCP servers and no `.claude/rules/`. Baz reads skills and agents at the repository root
only, so a [`[[scopes]]`](monorepo.md) run writes only the scope's `AGENTS.md`. An item with `targets` is written when a
target names `baz` or `AGENTS.md`.

## Options

All keys go in `[rules]`:

```toml
[rules]
baz_scoped = "nested"   # nested | root
baz_default = "all"     # all | none
baz_rest = "root"       # root | skip
baz_unscoped = "root"   # root | skip
baz_dedupe = false
baz_dedupe_max = 0
baz_budget = 0

[rules.baz]
include = ["db-dao"]    # with baz_default = "none"
```

| Key | Type | Default | Meaning |
| --- | ---- | ------- | ------- |
| `baz_scoped` | string | `"nested"` | `nested` writes path-scoped items to the `AGENTS.md` of their directory; `root` keeps them in the root `AGENTS.md` with an `_Applies to:_` line |
| `baz_default` | string | `"all"` | `all` nests every path-scoped item; `none` nests only items that opt in |
| `rules.baz.include` | list | `[]` | Rule and context names that opt in under `baz_default = "none"` (case-insensitive, with or without `.md`) |
| `baz_rest` | string | `"root"` | `skip` leaves path-scoped items that `baz` does not select out of the root `AGENTS.md` |
| `baz_unscoped` | string | `"root"` | `skip` leaves path-scoped items with no directory out of the root `AGENTS.md` |
| `baz_dedupe` | boolean | `false` | Write an item that points into several sibling directories once, to their parent's `AGENTS.md` |
| `baz_dedupe_max` | integer | `0` (off) | Collapse an item that would be copied into more than this many directories to the directory covering all of them |
| `baz_budget` | integer | `0` (off) | Warn when the Baz `AGENTS.md` files together exceed this many bytes |

Only the Baz `AGENTS.md` files change with these keys; the output of the other presets is the same in every mode.

## Path-scoped rules

With `baz_scoped = "nested"`, a rule or context item with `paths` or `globs` goes to the `AGENTS.md` of the directory
its globs point into. The directory is the leading wildcard-free part of each positive glob: `{a,b}` is expanded,
several directories get a copy each, and a directory below another listed one is covered by it. The scope of a nested
file is the whole directory, which can be wider than the glob; the `_Applies to:_` line keeps the exact globs.

The item stays in the root `AGENTS.md` when:

- a positive glob has no directory (`**/*.py`, `*.md`), or only negated globs are set;
- the directory does not exist, is a `[[scopes]]` path, is inside the configuration directory or `.git`, belongs to
  another ai-rulez root nested in the project (`generate` warns and names the rule), or is outside the project;
- `baz_scoped = "root"`.

An item that moves to a nested file is left out of the root `AGENTS.md` for every preset that writes it, so the
shared file stays identical.

### Path-scoped rules with no directory

A rule scoped by `**/BUILD`, `**/*.go` or `.mcp.json` names no directory, so it cannot be nested and the root
`AGENTS.md` inlines it with its `_Applies to:_` line. Any harness that reads the root file then loads that rule in every
session, on top of its own copy in a rules folder (`.claude/rules/`, `.cursor/rules/`). `baz_unscoped = "skip"` leaves
such rules out of the root file.

`skip` applies only while `baz` is the only preset writing the root `AGENTS.md` and `baz_scoped = "nested"`. When
`codex`, `opencode`, `amp` or another preset that reads `AGENTS.md` is configured, they need the rule, so the file keeps
it.

### Rules Baz does not select

With `baz_default = "none"`, rules that do not opt in, and rules excluded with `!baz`, are not nested, so the root
`AGENTS.md` keeps inlining them. `baz_rest = "skip"` leaves them out of the root file. Like `baz_unscoped`, it applies
only while `baz` is the only preset writing the root `AGENTS.md` and `baz_scoped = "nested"`. When `baz_budget` is
exceeded, the warning also names the path-scoped rules the root file inlines.

## Narrowing the nested surface

A rule with many globs is copied into every directory they point into, so a large monorepo can grow one nested
`AGENTS.md` per service.

- **`baz_default = "none"`** nests only items that opt in. The others stay in the root `AGENTS.md`, which Baz also
  reads, so nothing is lost; it is only not duplicated into the directory files.
- **`baz_dedupe = true`** writes an item whose globs point into several sibling directories once, to the parent's
  `AGENTS.md` (`services/AGENTS.md` instead of `services/api/AGENTS.md` and `services/web/AGENTS.md`).
- **`baz_dedupe_max = <K>`** collapses an item that would be copied into more than K directories to the directory
  covering all of them, even without `baz_dedupe`. When they share only the repository root (`platform/`, `deploy/`,
  `tools/`), the item goes to the root `AGENTS.md`; with `baz_unscoped = "skip"` the root is off limits and the copies
  stay.
- **`baz_budget = <bytes>`** makes `generate` warn when the root and nested Baz `AGENTS.md` files together exceed the
  budget, naming the largest files. It never fails the run.

### Per-rule opt-in and opt-out

- **`targets: ["!baz"]`** in a rule's frontmatter excludes it from everything `baz` writes on its own: the nested
  `AGENTS.md` files, skills and agents. A negated target wins over a positive one, and a rule whose targets are all
  negated is still written for every other preset. `!baz` does not remove a rule from the root `AGENTS.md`, which other
  presets share; restrict the rule with targets that name those presets instead.
- **`[rules.baz] include`** opts rules in by name under `baz_default = "none"`. Under the default `"all"` it makes no
  difference.

```toml
[rules]
baz_default = "none"

[rules.baz]
include = ["dao"]
```

`baz: true` in a rule's metadata opts it in the same way, but `validate` reports it as an unknown key (`AR303
frontmatter-key-unknown`). Prefer `include`, or add `allowed_keys = ["baz"]` to `[lint]` to silence the finding:

```markdown
<!-- .ai-rulez/rules/dao.md -->
---
type: Decision
title: DAO conventions
x-ai-rulez:
  kind: rule
  id: dao
  metadata:
    paths:
      - "services/db/**/*.go"
    baz: true
---

Every query goes through the DAO layer.
```

## Using it with other presets

- **`claude`**: Baz reads `.claude/skills/` and `.claude/agents/`, so with `claude` configured `baz` writes neither,
  and nothing is read twice. Claude Code's commands are written as user-invocable skills under `.claude/skills/`,
  which Baz reads as skills. With the default `CLAUDE.md` (which imports `@AGENTS.md`), Claude Code loads only the root
  `AGENTS.md`, not the nested ones; without a `CLAUDE.md` (`[claude] claude_md = "none"`), Claude Code can also load a nested
  `AGENTS.md` when it reads a file in that directory, so a scoped rule can reach it twice. Limit such a rule with
  `targets`.
- **`codex`, `opencode`, `amp`, `xum`** and the other `AGENTS.md` presets share the root `AGENTS.md`. With
  `agents_md = true` (the default) it and `.agents/skills/` are written once, and `baz` adds its nested files.
- **`cursor`** and other presets that write `.cursor/rules`: Baz reads those files too, so a rule can reach it from
  both `.cursor/rules` and `AGENTS.md`. Baz applies every in-scope guideline, so contradictory rules produce
  contradictory feedback.

## Limits

- Nothing reaches Baz until it is on the default branch.
- The preset cannot change Baz's UI settings (draft reviews, ignored branches, custom reviewers); an organization
  admin configures those.
- Baz does not read nested `.claude/` and `.cursor/` directories. `baz` never writes them, but another preset in a
  `[[scopes]]` run can.
- Baz reads committed files only, so machine-local configuration has no effect on it.
