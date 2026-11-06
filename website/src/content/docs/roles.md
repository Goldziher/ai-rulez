---
title: "Roles"
description: "A role maps a job to the slice of the shared content a person needs: which domains, which skills, rules, agents, commands and checks, and how each skill is surfaced."
---

A role maps a job to the slice of the shared content a person needs: which domains, which skills, rules, agents,
commands and checks, and how each skill is surfaced. `generate --role <name>` renders that slice, for the project or
with `--user` for one person's machine. Use roles when one repository serves people with different jobs, such as
backend engineers, release managers and support agents.

:::note[ai-rulez does not do identity]
A role is a name. ai-rulez has no notion of users, groups or authentication. Something else (an identity-provider
integration, a web UI, a wrapper script) decides who holds which role and runs `ai-rulez generate --role <name>`.
The `match` hints on a role are strings for that tool; ai-rulez never interprets them.
:::

## Example

```toml
# .ai-rulez/config.toml
presets = ["claude", "cursor", "copilot"]

[role_manifest]
enabled = true                  # write .ai-rulez/roles.json on generate

[[roles]]
name = "engineer"
description = "Everyone who writes code"
domains = ["backend"]

[roles.skills]
exclude = ["deploy-*"]

[roles.skill_mode]
"review-*" = "name-only"

[roles.match]
groups = ["okta:eng"]

[[roles]]
name = "release-manager"
extends = "engineer"
domains = ["release"]

[roles.skill_mode]
deploy = "user-invocable-only"
```

The `backend` domain has the skills `review-pr` and `deploy-staging`; the `release` domain has `deploy`.

```bash
ai-rulez roles resolve release-manager
```

```text
role release-manager: 4 item(s), 895 bytes, ~237 tokens (cl100k_base)
KIND   DOMAIN   ID               MODE                 DELIVERY  BYTES  TOKENS
rule   -        commit-messages  -                    -         153    42
rule   backend  database         -                    -         156    43
skill  backend  review-pr        name-only            static    289    78
skill  release  deploy           user-invocable-only  static    297    74

skill_mode not honored on every configured harness (see https://goldziher.github.io/ai-rulez/roles/#skill_mode-on-other-harnesses):
  backend/review-pr = name-only: not applied on copilot, cursor; the skill stays listed there
```

`deploy-staging` is excluded by the parent role. `ai-rulez generate --role release-manager` writes the two skills
and sets their modes for each tool. Claude Code gets them as `skillOverrides` in `.claude/settings.json`:

```json
{
  "skillOverrides": {
    "deploy": "user-invocable-only",
    "review-pr": "name-only"
  }
}
```

Cursor and Copilot read the shared `.agents/skills/`, where `user-invocable-only` becomes a frontmatter key:

```markdown
<!-- .agents/skills/deploy/SKILL.md -->
---
name: deploy
description: "Use when deploying to production."
disable-model-invocation: true
---
```

`name-only` has no equivalent there, so `review-pr` stays fully listed and `generate` warns.

## Profiles vs roles

Both select a slice of the same content, but they answer different questions.

|              | Profile                            | Role                                       |
| ------------ | ---------------------------------- | ------------------------------------------ |
| Question     | what does this repository ship?    | what does this person's job need?          |
| Scope        | the project or team                | a person (identity decided elsewhere)      |
| Selected by  | `--profile`, `default`             | `--role`                                   |
| Extra knobs  | none                               | per-kind selectors, `skill_mode`, `delivery`, `match`, `pin` |
| Compared by  | `tokens --compare-profiles`        | `tokens --by-role`                         |

Use a [profile](/ai-rulez/domains/) when one checkout emits several project variants (a backend service, a frontend app) and
that choice belongs in the repository. Use a role when the same content is tailored to different people at
generation time. Domains are the shared vocabulary: profiles group domains for a project, roles group domains for a
person. Many projects keep a `default` profile for the repository and add roles for per-person variants.

## Defining roles

Roles are a `[[roles]]` array in `config.toml`. Manifest settings live in a separate `[role_manifest]` table, because
TOML cannot use `roles` as both an array and a table.

| Key                                               | Type   | Meaning                                                                                                   |
| ------------------------------------------------- | ------ | --------------------------------------------------------------------------------------------------------- |
| `name`                                            | string | Lowercase letters, digits, `-` and `_`, starting with a letter or digit; unique. What `--role` takes.   |
| `description`                                     | string | Shown by `roles list`.                                                                                    |
| `domains`                                         | list   | Domains the role selects, as in a profile (`builtin:<name>` works too). Root content, globally enabled builtins and include-sourced domains are always kept. |
| `skills`, `rules`, `agents`, `commands`, `checks` | table  | `include` and `exclude` lists of item ids or [`path.Match`](https://pkg.go.dev/path#Match) globs. An entry with `/` matches `domain/id`. An empty `include` keeps everything the domains provide; `exclude` always wins. Context has no selector: context files always stay. |
| `skill_mode`                                      | table  | Skill id or glob to `on`, `name-only`, `user-invocable-only` or `off`.                                    |
| `delivery`                                        | table  | Skill id or glob to `static`, `served` or `both`; see [Delivery](#delivery).                              |
| `extends`                                         | string | One parent role.                                                                                          |
| `match`                                           | table  | `groups`: hint strings for an external tool. Not inherited.                                              |
| `pin`                                             | bool   | Record the digest of the role's rendered outputs in `ai-rulez.lock`. Not inherited.                       |

| `[role_manifest]` key | Default | Meaning                                                                              |
| --------------------- | ------- | ------------------------------------------------------------------------------------ |
| `enabled`             | `false` | write `.ai-rulez/roles.json` on `generate`; see [The roles manifest](#the-roles-manifest) |
| `skill_mode_fallback` | `drop`  | what `off` does where a tool cannot hide a skill; see [`skill_mode` on other harnesses](#skill_mode-on-other-harnesses) |

Roles can be overlaid in `config.local.toml` like profiles: entries merge by `name`, and `remove = true` drops a
shared role on your machine.

:::caution[A role is a content filter, not an access boundary]
A role narrows the rules, skills, agents, commands and checks that are rendered or served, and sets skill modes.
It does not narrow hooks, `[permissions]`, `[[mcp_servers]]` or context files: those are rendered for every role.
Do not use a role to withhold a tool or a credential; the outputs are plain files the holder can read and edit.
:::

### `skill_mode` precedence

For one skill, every matching key competes: an exact id beats a glob, a longer glob beats a shorter one, and a tie
goes to the lexically first pattern. A skill no key matches keeps the default, `on`.

Claude Code's `skillOverrides` is keyed by skill id, so two kept skills with the same id in different domains share
one entry. When the role gives them different modes, `validate` reports `AR971`; give both the same mode (a bare id
key does) or exclude one.

### Inheritance

`extends` is one level deep: a role whose parent extends another role is an error. The child is merged over the
parent:

| Field                    | Merge                                                 |
| ------------------------ | ----------------------------------------------------- |
| `domains`                | union, parent first                                   |
| `exclude` lists          | union                                                 |
| `include` lists          | union when both set one, otherwise whichever is set   |
| `skill_mode`, `delivery` | merged; the child wins per key                        |
| `description`            | the child's, else the parent's                        |
| `match`, `pin`           | not inherited                                         |

### Delivery

`[roles.delivery]` decides per skill whether the role's agent gets it as a file in the tool's skill folder (`static`),
only on demand from the [skills server](/ai-rulez/mcp-server/#dynamic-skill-loading) (`served`), or both. Keys follow the
`skill_mode` rules, and entries are inherited with the child winning.

```toml
[[roles]]
name = "backend"
domains = ["backend"]

[roles.delivery]
"deploy-*" = "served"          # not listed in the agent's context; found with find_skill
"backend/runbooks" = "both"
```

For one skill, the first that is set wins: its own `delivery` frontmatter, the role, `[domains.<name>] delivery`,
`[skills] delivery`, then `static`. Everything that renders or serves a role uses it:

- `generate --role backend` leaves served skills out of the skill folders and adds the `dynamic-skills` skill;
- `tokens --role backend` does not count served skills in the listing and names them;
- `ai-rulez mcp --serve-skills --role backend` serves them, with `find_skill` scoped to the role;
- `roles resolve`, `roles.json` and `catalog` report each skill's delivery;
- `ai-rulez lock` pins the served skills of every role, so `[lock] enforce` covers them.

## Commands

```bash
ai-rulez roles list [--format json]           # every role with item counts and token estimates
ai-rulez roles show <name> [--format json]    # as declared, and with the parent merged in
ai-rulez roles resolve <name> [--format json] # the items the role keeps, with sizes, modes and delivery

ai-rulez generate --role engineer             # project outputs for the role
ai-rulez generate --user --role engineer      # user-level outputs (~/.claude, ~/.codex, ...)
ai-rulez generate --check --role engineer     # drift check against the role's outputs

ai-rulez tokens --role engineer               # token surface of the role
ai-rulez tokens --by-role                     # one comparison column per role
```

- `--role` replaces profile selection and cannot be combined with `--profile` or `--plugin`. Everything in the run
  (outputs, token report, usage index) sees the role's slice.
- Installed skills scoped to profiles are not filtered by a role, and the machine-local `local/` tree is not
  narrowed.
- `generate --role` prints the role's `AR971` findings as warnings before it writes anything, so a typo in a domain or
  an `exclude` entry is visible.

## Composing roles

`--role` takes a comma-separated list, as do `roles show`, `roles resolve`, `tokens --role` and
`mcp --serve-skills --role`. A composition is the union of its members: the domains and items they keep are combined,
and that union is rendered, served and counted.

```bash
ai-rulez generate --role engineer,oncall
ai-rulez roles resolve engineer,oncall
```

Where two members set `skill_mode` or `delivery` for the same skill, the [precedence](#skill_mode-precedence) applies
first (the more specific key wins); between equally specific keys, the role listed last wins. So `engineer,oncall`
and `oncall,engineer` can differ.

A composition is a value, not a `[[roles]]` entry: it cannot be extended, need not be declared, and every member must
exist (`AR971` otherwise). It can be pinned: `ai-rulez lock --role engineer,oncall` records the union's outputs under
the name `engineer,oncall`, and `generate --locked --role engineer,oncall` compares them. Pinning a member does not
pin the compositions that contain it, and the reverse.

## `skill_mode` becomes Claude Code `skillOverrides`

| Mode                  | Claude Code behavior                                         |
| --------------------- | ------------------------------------------------------------ |
| `on`                  | listed with its description (the default)                   |
| `name-only`           | listed by name only, saving description tokens               |
| `user-invocable-only` | hidden from the model; you can still run it as `/name`      |
| `off`                 | hidden from the model and from you                           |

For every kept skill with a mode, `generate` writes `skillOverrides.<skill>` in `.claude/settings.json` (or
`~/.claude/settings.json` with `--user`) with the same per-key ownership as
[`[claude.settings.managed]`](/ai-rulez/settings/):

- Only the skill ids the role lists are owned; other entries are left alone, and `clean` removes exactly what was
  written. A role's modes win over the same skill in `[claude.settings.managed] skill_overrides`.
- If you had set a different value for one of those ids by hand, `generate --role` warns, replaces it, and keeps
  yours in `<config dir>/local/.role-skill-overrides.json` (machine-local, gitignored). A plain `generate`, or a role
  that no longer lists the skill, restores your value.
- A value you change after the role wrote it is yours and is left alone.
- Switching roles removes the first role's entries and writes the second's. `mcp --serve-skills --role` never writes
  the settings file.

## `skill_mode` on other harnesses

Only Claude Code has a per-skill settings key. For other tools, ai-rulez writes the mode into the skill's `SKILL.md`
frontmatter (or Codex's `agents/openai.yaml`) where the vendor documents an equivalent. A key you set in the skill's
own frontmatter is never overwritten.

| Preset          | `user-invocable-only`                                            | `off`                                                         | `name-only` |
| --------------- | ---------------------------------------------------------------- | ------------------------------------------------------------- | ----------- |
| `claude`        | `skillOverrides`                                                 | `skillOverrides`                                              | `skillOverrides` |
| `cursor`        | `disable-model-invocation: true`                                 | no documented setting                                         | no          |
| `codex`         | `agents/openai.yaml` `policy.allow_implicit_invocation: false`   | not rendered (Codex needs an absolute path)                   | no          |
| `copilot`       | `disable-model-invocation: true`                                 | `disable-model-invocation: true` and `user-invocable: false`  | no          |
| `opencode`      | no                                                               | not rendered (`permission.skill` in `opencode.json`)          | no          |
| `gemini`        | no                                                               | no settings key (only the `/skills disable` command)          | no          |
| other presets   | not documented                                                   | not documented                                                | not documented |

Where a cell says "no" or "not documented", the skill stays listed on that tool. `roles resolve` and `doctor` list
every skill and tool where a mode is not honored, and `generate` warns.

`off` hides a skill only when every configured tool can hide it. Otherwise `generate --role` follows
`skill_mode_fallback`:

```toml
[role_manifest]
skill_mode_fallback = "drop"   # drop (default) | serve
```

`drop` leaves the skill out of the role's render for every tool of the run, Claude Code included. `serve` switches it
to [served delivery](#delivery), so tools that can call MCP find it with `find_skill`.

## Strict validation

| Code                                                    | Meaning                                                                                                                          |
| ------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| [`AR971`](/ai-rulez/strict-validation/) `role-reference-unknown` | A domain that does not exist, or a selector, `skill_mode` or `delivery` entry that matches no item (or only items in a domain the role does not select). |
| [`AR972`](/ai-rulez/strict-validation/) `role-extends-invalid`   | Unknown parent, cycle, or inheritance deeper than one level. Such a role is left out of `roles.json`.                           |
| [`AR973`](/ai-rulez/strict-validation/) `role-unreachable-dependency` | An item the role keeps names, in its `skills:` frontmatter, a skill the role drops or hides with `off` or `user-invocable-only`. |

A bad name, a duplicate name or an invalid `skill_mode` or `delivery` value fails loading outright.

## The roles manifest

With `[role_manifest] enabled = true`, `generate` writes `.ai-rulez/roles.json`; `roles list --format json` prints the
same document. It has no timestamps and is sorted (roles by name, items by kind, domain and id), so it is safe to
commit. It is built from the shared sources only: roles from `config.local.toml` and items under `local/` never
appear in it, so the file is the same on every machine. The schema is
[`schema/roles-manifest.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/roles-manifest.schema.json).

```json
{
  "schema_version": 1,
  "tokenizer": "cl100k_base",
  "roles": [
    {
      "name": "engineer",
      "description": "Everyone who writes code",
      "match": { "groups": ["okta:eng"] },
      "domains": ["backend"],
      "skill_modes": { "review-pr": "name-only" },
      "items": [
        { "kind": "rule", "id": "commit-messages", "path": "rules/commit-messages.md", "bytes": 153, "tokens": 42 },
        ...
      ],
      "totals": {
        "items": 3, "bytes": 598, "tokens": 163, "by_kind": { "rule": 2, "skill": 1 },
        "served_skills": 0, "served_tokens": 0
      }
    }
  ]
}
```

| Field                            | Meaning                                                                                       |
| -------------------------------- | --------------------------------------------------------------------------------------------- |
| `items[].bytes`                  | size of the item's source files on disk (a skill counts its resources)                        |
| `items[].tokens`                 | estimate for the primary file (`SKILL.md`, the rule file), like every `ai-rulez tokens` figure |
| `items[].mode`, `items[].delivery` | the skill's resolved mode and delivery                                                       |
| `items[].owner`, `items[].version` | from the item's frontmatter, when set                                                        |
| `delivery`                       | the role's `[roles.delivery]` table                                                           |
| `totals.served_skills`, `totals.served_tokens` | skills that are served only, which cost no listing tokens until loaded             |

## Integrating an identity tool or UI

Everything an integration needs is in versioned `--format json` output. ai-rulez never calls the network, and an
integration never parses config files.

1. **Discover the roles.** Read the committed `roles.json`, or run `ai-rulez roles list --format json`. Each role has
   its `name`, `description` and `match.groups`.
2. **Map a person to a role.** This is your tool's job: for example, read the person's identity-provider groups and
   pick the role whose `match.groups` contains one of them, by your own precedence when several match.
3. **Preview.** `ai-rulez roles resolve <name> --format json` lists the items the role keeps, with owner, version,
   bytes and tokens. `ai-rulez tokens --role <name> --format json` reports the token surface.
4. **Apply.** Run `ai-rulez generate --user --role <name> --yes` (user level) or `ai-rulez generate --role <name>`
   (project level). Exit code 0 means the files were written.
5. **Show the catalog.** `ai-rulez catalog --format json` lists every item with owner, version, size, digest, the
   roles that keep it and the lock status. See [Catalog](/ai-rulez/catalog/) and
   [`schema/catalog.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/catalog.v1.schema.json).
6. **Audit.** [`ai-rulez lock --diff --format json`](/ai-rulez/lockfile/) reports what changed between the committed lock and
   the working tree.

Every JSON document carries `schema_version`; refuse a version you do not know.

## Roles and the lock file

The [lock file](/ai-rulez/lockfile/) pins each role declaration (`kind = "role"`), so adding a role or changing what it
includes shows up in `lock --diff` and fails `lock --check`. A role's rendered outputs are pinned, as one digest per
role, when the role sets `pin = true` or with `lock --roles`. `lock --check --role <name>` and
`generate --check --locked --role <name>` then catch a change in what the role renders even when the sources are the
same (a new `skill_mode` entry, a new ai-rulez release); see [Composing with roles](/ai-rulez/lockfile/#composing-with-roles).
Without a pin, `generate --locked --role <name>` still verifies that the sources match the lock. Served skills are
pinned as `[[served]]` entries; see [Served skills and skill sources](/ai-rulez/lockfile/#served-skills-and-skill-sources).
