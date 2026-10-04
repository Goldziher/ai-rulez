# Skill Frontmatter

A skill is `.ai-rulez/skills/<id>/SKILL.md`: YAML frontmatter followed by the instructions. `generate` carries the
frontmatter into each preset's skill directory **with its original types**. A nested `metadata:` map stays a map,
`disable-model-invocation: true` stays a boolean, and `last_verified: 2026-10-01` stays a date. Versions before 4.25.0
wrote `metadata: map[owner:team-a ...]`, `"true"` and `2026-10-01 00:00:00 +0000 UTC`.

```yaml
---
name: release-notes
description: Use when writing release notes for a tag.
license: MIT
compatibility: Needs git and gh
allowed-tools: Bash(git:*) Read
disable-model-invocation: true
paths:
  - "CHANGELOG.md"
metadata:
  owner: team-a
  reviewed: { by: alice, date: 2026-10-01 }
  tags: [docs, release]
---
```

The parsed YAML of `license`, `compatibility`, `allowed-tools`, `metadata`, `disable-model-invocation` and any other
key is equal in source and output. Collections are written in block style, aliases are resolved, comments are dropped,
and keys are sorted, so output is deterministic.

## What each preset writes

| Key                                                      | Claude Code (`.claude/skills`)        | Cursor (`.agents/skills`) | Every other preset's skill tree                 |
| -------------------------------------------------------- | ------------------------------------- | ------------------------- | ----------------------------------------------- |
| `name`, `description`                                    | yes                                   | yes                       | yes                                             |
| `license`, `compatibility`, `metadata`, `allowed-tools`  | yes                                   | yes                       | yes (the Agent Skills specification fields)     |
| `paths` (or `globs`)                                     | yes, gates the skill to matching files | yes                      | no: no other tool documents a skill `paths` key |
| `disable-model-invocation`                               | yes                                   | yes                       | Codex: `agents/openai.yaml`, see below          |
| `user-invocable`                                         | yes                                   | no                        | no                                              |
| any other key (`model`, `context`, `hooks`, `argument-hint`, ...) | yes                          | no                        | no                                              |

"Every other preset" means Codex, Copilot, OpenCode, Gemini, Windsurf, Cline, Antigravity, Xum, Baz and the shared
`.agents/skills` tree of `agents_md`. Keys a tool does not document are not written to its tree: some consumers reject
unknown keys (Claude.ai uploads and the Skills API accept only `name`, `description`, `license`, `compatibility`,
`metadata` and `allowed-tools`). Claude Code receives every key, like the `generate --plugin` bundle, which copies
`SKILL.md` verbatim.

Vendor facts behind the table (verified 2026-10-04): Claude Code documents `paths`, `disable-model-invocation`,
`user-invocable`, `allowed-tools`, `license`, `compatibility` (up to 500 characters) and `metadata` as skill frontmatter
and ignores unknown fields; the [Agent Skills specification](https://agentskills.io/specification) defines `name`
(64 characters), `description` (1024 characters), `license`, `compatibility`, `metadata` (string keys and values) and
`allowed-tools` (space-separated string); Cursor documents `paths` (with `globs` as a legacy fallback),
`disable-model-invocation` and `metadata`.

## Invocation keys

`user-invocable` and `disable-model-invocation` are decided by the author. ai-rulez never overwrites a value you set;
`yes`, `no`, `on`, `off`, `1` and `0` are accepted and written as a boolean.

- **Default**: a skill is user-invocable (Claude Code's default), so the key is not written and the skill appears in
  the `/` menu. Before 4.25.0 the `claude` preset wrote `user-invocable: false` on every skill, which, now that the key
  is spelled correctly, hid all skills from the menu.
- **Hide all skills from the menu**: `[claude.skills] hide_from_menu = true` writes `user-invocable: false` on skills that
  do not set the key. A skill with its own `user-invocable: true` stays visible.
- **Commands** are rendered as skills and keep `user-invocable: true` unless the command sets the key.
- `argument-hint` is passed through on skills again (it is a documented skill field and skills are user-invocable).
  The warning that called it inert was removed.
- **Codex** has no `SKILL.md` key for this. `disable-model-invocation: true` writes
  `.agents/skills/<id>/agents/openai.yaml` with `policy: { allow_implicit_invocation: false }`, so Codex runs the skill
  only when it is named with `$skill`.

## Path-gated skills

`paths` (or `globs`) on a skill makes Claude Code and Cursor load it only when matching files are in play. Patterns use
the same syntax as rule `paths`. The other tools ignore the key.

## Size limits

`generate` and `tokens` warn when a root instruction file exceeds a limit the tool documents. See
[Rules: size limits](rules.md#size-limits).
