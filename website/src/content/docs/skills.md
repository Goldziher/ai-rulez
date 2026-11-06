---
title: "Skill frontmatter"
description: "A skill is a directory .ai-rulez/skills/<id>/ (or .ai-rulez/domains/<domain>/skills/<id>/) with a SKILL.md: YAML frontmatter followed by the instructions, plus optional references/, scripts/ and assets/ folders."
---

A skill is a directory `.ai-rulez/skills/<id>/` (or `.ai-rulez/domains/<domain>/skills/<id>/`) with a `SKILL.md`:
YAML frontmatter followed by the instructions, plus optional `references/`, `scripts/` and `assets/` folders.
`generate` copies each skill into the skill folder of every enabled tool and carries the frontmatter over with its
original YAML types: a nested `metadata` map stays a map, `disable-model-invocation: true` stays a boolean.

## Example

```text
.ai-rulez/skills/changelog-draft/
├── SKILL.md
└── scripts/
    └── changes.sh
```

```markdown
<!-- .ai-rulez/skills/changelog-draft/SKILL.md -->
---
type: Playbook
title: Changelog Draft
description: Use when writing release notes for a tag.
x-ai-rulez:
  kind: skill
  id: changelog-draft
  metadata:
    name: changelog-draft
    license: MIT
    allowed-tools: Bash(git:*) Read
    disable-model-invocation: true
    argument-hint: "<tag>"
    paths:
      - CHANGELOG.md
    metadata:
      owner: docs-team
      tags: [docs, release]
---

# Changelog draft

1. Run `scripts/changes.sh <tag>` to list the merged pull requests.
2. Group them under Features, Fixes and Chores.
```

`ai-rulez add skill <name>` creates the directory with a `SKILL.md` (and its `index.md` listing); add `scripts/`,
`references/` or `assets/` yourself. As for [rules](/ai-rulez/rules/#rule-files), `type`, `title` and `description` are the
OKF keys of the file and the ai-rulez keys go under `x-ai-rulez.metadata`; they also work at the top level. `type` is
required: a hand-written `SKILL.md` without it is an `AR9B1` error, and a hand-made skill directory is reported as
unlisted (`AR9B0`) until `ai-rulez generate` refreshes the `index.md` listings.

```toml
# .ai-rulez/config.toml
presets = ["claude", "cursor", "codex"]
```

`ai-rulez generate` writes `.claude/skills/changelog-draft/` for Claude Code and one shared
`.agents/skills/changelog-draft/` that Cursor and Codex both read. Each gets `SKILL.md` and `scripts/changes.sh`
(executable bit kept). Claude Code receives every key:

```markdown
<!-- .claude/skills/changelog-draft/SKILL.md -->
---
allowed-tools: Bash(git:*) Read
argument-hint: <tag>
description: Use when writing release notes for a tag.
disable-model-invocation: true
license: MIT
metadata:
    owner: docs-team
    tags:
        - docs
        - release
name: changelog-draft
paths:
    - CHANGELOG.md
# Content-Hash: blake3:9aae4dd1...
---

# Changelog draft

1. Run `scripts/changes.sh <tag>` to list the merged pull requests.
2. Group them under Features, Fixes and Chores.


## Resources

This skill bundles supporting files. Read them on demand when the task calls for them — don't bulk-load.

### Scripts

- `scripts/changes.sh`
```

The shared tree gets the portable keys only, so `argument-hint` is left out:

```markdown
<!-- .agents/skills/changelog-draft/SKILL.md -->
---
name: changelog-draft
description: "Use when writing release notes for a tag."
allowed-tools: Bash(git:*) Read
disable-model-invocation: true
license: MIT
metadata:
  owner: docs-team
  tags:
    - docs
    - release
paths:
  - CHANGELOG.md
# Content-Hash: blake3:9aae4dd1...
---
...
```

## Frontmatter keys

| Key                        | Type    | Meaning                                                                                       |
| -------------------------- | ------- | --------------------------------------------------------------------------------------------- |
| `name`                     | string  | The skill name. Defaults to the directory name.                                               |
| `description`              | string  | What the skill does and when to use it. Tools list it to decide when to load the skill.       |
| `license`                  | string  | [Agent Skills](https://agentskills.io/specification) field.                                   |
| `compatibility`            | string  | Agent Skills field: environment requirements (`Needs git and gh`).                            |
| `metadata`                 | map     | Agent Skills field: free-form keys. The spec expects string values; nested values are passed through as written. |
| `allowed-tools`            | string  | Agent Skills field: space-separated tools the skill may use.                                  |
| `paths` (or `globs`)       | list    | Load the skill only when matching files are in play; see [Path-gated skills](#path-gated-skills). |
| `disable-model-invocation` | boolean | `true`: only the user can invoke the skill; see [Invocation keys](#invocation-keys).          |
| `user-invocable`           | boolean | `false`: hide the skill from the `/` menu.                                                    |
| `delivery`                 | string  | `static`, `served` or `both`; see [Dynamic loading](#dynamic-loading-delivery). Never written to output. |
| `priority`, `targets`      |         | As for [rules](/ai-rulez/rules/#frontmatter): ordering and the outputs the skill is written to.       |
| any other key              |         | Passed through to Claude Code (`model`, `context`, `hooks`, `argument-hint`, ...).            |

Output frontmatter is deterministic: collections are written in block style, aliases are resolved and comments are
dropped. In `.claude/skills/` all keys are sorted; in the other skill trees `name` and `description` come first and
the rest are sorted.

## What each preset writes

| Key                                                     | Claude Code (`.claude/skills/`) | Every other skill tree (`.agents/skills/`, `.github/skills/`, ...) |
| ------------------------------------------------------- | ------------------------------- | ------------------------------------------------------------------ |
| `name`, `description`                                   | yes                             | yes                                                                |
| `license`, `compatibility`, `metadata`, `allowed-tools` | yes                             | yes                                                                |
| `paths`                                                 | yes                             | yes (read by Cursor; ignored by tools without a skill path gate)   |
| `disable-model-invocation`, `user-invocable`            | yes                             | yes                                                                |
| `delivery`                                              | no                              | no                                                                 |
| any other key                                           | yes                             | no                                                                 |

Other tools get only the keys above because some consumers reject unknown keys (Claude.ai uploads and the Skills API
accept only the Agent Skills fields). The [`generate --plugin`](/ai-rulez/plugins/) bundle copies `SKILL.md` as written.

## Invocation keys

`user-invocable` and `disable-model-invocation` are yours: ai-rulez never overwrites a value you set. Use a YAML
boolean; another value (`"no"`, `"off"`) is reported as `AR304`.

- By default a skill is user-invocable, the key is not written, and the skill appears in the `/` menu.
- `[claude.skills] hide_from_menu = true` writes `user-invocable: false` into every Claude Code skill that does not
  set the key. A skill with its own `user-invocable: true` stays visible.
- Commands rendered as skills keep `user-invocable: true` unless the command sets the key.
- Codex has no `SKILL.md` key for this. When the `codex` preset writes its own skill tree (`agents_md = false`),
  `disable-model-invocation: true` adds `agents/openai.yaml` with `policy: { allow_implicit_invocation: false }`, so
  Codex runs the skill only when you name it with `$changelog-draft`.

```toml
[claude.skills]
hide_from_menu = true
```

## Path-gated skills

`paths` (or `globs`) makes Claude Code and Cursor load a skill only when files matching the globs are in play. The
syntax is the same as for rule [`paths`](/ai-rulez/rules/#frontmatter). Tools without a skill path gate ignore the key and
list the skill as usual.

## Hooks that run bundled scripts

A skill's `hooks` key (Claude Code) runs commands while the skill is active. Claude Code sets `${CLAUDE_SKILL_DIR}` to
the skill's own directory, so a hook can reach a bundled script from any working directory:

```yaml
hooks:
  PostToolUse:
    - matcher: Edit
      hooks:
        - type: command
          command: "${CLAUDE_SKILL_DIR}/scripts/check.sh"
```

`validate` resolves the path against the skill directory: a missing file is `AR501`, and a script run
directly (not through `bash` or another interpreter) without the executable bit is `AR502`. The variable is defined
only for skills, not for agent or command hooks.

## Dynamic loading (`delivery`)

`delivery` decides how a skill reaches the agent:

| Value              | Effect                                                                                     |
| ------------------ | ------------------------------------------------------------------------------------------ |
| `static` (default) | written into every tool's skill folder                                                     |
| `served`           | left out of the skill folders and served on demand by `ai-rulez mcp --serve-skills`        |
| `both`             | written to the folders and served                                                          |

```yaml
# .ai-rulez/skills/db-migrations/SKILL.md, under x-ai-rulez.metadata
delivery: served
triggers: [schema change, alembic migration, add a column]
```

When any skill is served, each tool's skill folder gets a `dynamic-skills` skill that tells the agent to call
`find_skill` and `load_skill`, and the default `ai-rulez` MCP server entry starts with `--serve-skills`.

The default comes from, in order: the skill's own `delivery`, the [role's](/ai-rulez/roles/#delivery) `delivery`,
`[domains.<name>] delivery`, `[skills] delivery`, then `static`. A project can serve a whole domain, or every skill,
and mark a few core skills `static`:

```toml
[skills]
delivery = "served"
```

Warnings to know:

| Code    | When                                                                                                         |
| ------- | ------------------------------------------------------------------------------------------------------------ |
| `AR990` | Always-loaded content (a rule, context, agent or command) names a `served` skill, which is not in the tool's skill folder. A served skill naming another served skill is fine. |
| `AR994` | A skill's `delivery` is not `static`, `served` or `both` (error). |
| `AR992` | A served skill targets a tool that cannot call MCP tools; it is written there as a static file.             |
| `AR993` | Skills are served but no MCP server entry runs `ai-rulez mcp --serve-skills`.                               |
| `AR9R0` | A `served` skill runs a bundled `scripts/` or `assets/` file by relative path. Nothing is on disk to run; use `delivery: both`. |
| `AR9R1` | Always-loaded content points at a `both` skill with `disable-model-invocation: true` by name; the model's Skill tool refuses it. Point at the file path instead. |
| `AR9R2` | A `served` skill sets `disable-model-invocation: true`: it is written nowhere and hidden from the model, so nobody can run it. Use `delivery: both`, or set `[mcp] serve_user_invoked_skills = true`. |

An MCP client that must keep a skill served can still fetch its files with `list_skill_resources` and
`read_skill_file`; see [Dynamic skill loading](/ai-rulez/mcp-server/#dynamic-skill-loading).

## Size limits

`generate` and `tokens` warn when a root instruction file exceeds a limit the tool documents. See
[Rules: size limits](/ai-rulez/rules/#size-limits).

## See also

- [Installed skills](/ai-rulez/installed-skills/): skills fetched from other repositories.
- [AGENTS.md and .agents/skills](/ai-rulez/agents-md/): the shared skill tree.
