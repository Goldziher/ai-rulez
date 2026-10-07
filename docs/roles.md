# Roles

A **profile** picks domains for a project. A **role** maps a *job* to the slice of the shared content a person
needs: which domains, which skills, rules, agents and commands, and how Claude Code should surface each skill.
Profiles answer "what does this repository ship"; roles answer "what does a backend engineer, a data analyst or
a support agent get on their machine".

!!! note "ai-rulez never does identity"
    ai-rulez has no notion of users, groups, authentication or the network. A role is a name. Something else (an
    identity-provider integration, a web UI, a wrapper script) decides which person holds which role and runs
    `ai-rulez generate --user --role <name>`. The `match` hints on a role are inert strings that tool may read
    from `roles.json`; ai-rulez stores and republishes them and never interprets them.

## Defining roles

Roles are a `[[roles]]` array in `config.toml`. The optional manifest switch lives in a separate
`[role_manifest]` table, because TOML cannot use `roles` as both an array and a table.

```toml
[role_manifest]
enabled = true                       # write .ai-rulez/roles.json on generate

[[roles]]
name = "engineer"
description = "Everyone who writes code"
domains = ["shared", "backend"]      # same meaning as a profile's list; "builtin:<name>" works too

[roles.skills]
exclude = ["deploy-*", "backend/release"]   # ids or path.Match globs; "domain/id" matches one domain

[roles.skill_mode]
"review-*" = "name-only"             # on | name-only | user-invocable-only | off
deploy = "user-invocable-only"

[roles.match]
groups = ["okta:eng", "okta:eng-platform"]  # free-form hints for an external identity tool

[[roles]]
name = "release-manager"
extends = "engineer"                 # one level only
domains = ["release"]

[roles.skill_mode]
deploy = "on"                        # child wins over the parent's "user-invocable-only"
```

| Field | Meaning |
| --- | --- |
| `name` | Lowercase letters, digits, `-` and `_`. What `--role` takes. Must be unique. |
| `description` | Free text shown by `roles list`. |
| `domains` | Domains the role selects. Root content, globally active builtins and included domains are always kept, exactly as for a profile. |
| `skills`, `rules`, `agents`, `commands`, `checks` | `include` and `exclude` lists. An entry is an item id or a [`path.Match`](https://pkg.go.dev/path#Match) glob; an entry containing `/` is matched against `domain/id`. An empty `include` keeps everything the domains provide; `exclude` always wins. There is no selector for `context`: context files stay. |
| `skill_mode` | Skill id or glob to a Claude Code `skillOverrides` state. |
| `delivery` | Skill id or glob to `static`, `served` or `both`: how the skill reaches this role's agent (see [Delivery](#delivery)). |
| `pin` | `true` records the digest of the role's rendered outputs in `ai-rulez.lock`. Not inherited. |
| `extends` | The name of one parent role. |
| `match` | `groups`: hint strings for an external tool. Not inherited. |

### `skill_mode` precedence

For one skill, every matching key competes: an exact id beats a glob, a longer glob beats a shorter one, and ties
go to the lexically first pattern. The answer never depends on map order. A skill no key matches keeps the
default, which is to render it with no override.

Claude Code's `skillOverrides` is one flat map keyed by skill id, so two kept skills with the same id in different
domains share one entry. When a role resolves them to different modes (for example `backend/deploy = "off"` while
`frontend/deploy` has no mode), `validate` reports `AR971`; give both the same mode (a bare `deploy` key does)
or exclude one of them.

### Delivery

`[roles.delivery]` decides, per skill, whether the role's agent gets the skill as a file in the harness skill
tree (`static`), only on demand from the [skills server](mcp-server.md#dynamic-skill-loading) (`served`), or both.
The keys follow the `skill_mode` rules (ids, globs, `domain/id`, the most specific key wins) and the entries are
inherited through `extends` with the child winning.

```toml
[[roles]]
name = "backend"
domains = ["backend"]

[roles.delivery]
"deploy-*" = "served"      # not listed in the backend agent's context, found with find_skill
"backend/runbooks" = "both"
```

Precedence for one skill: its own `delivery` frontmatter, then the role, then `[domains.<name>] delivery`, then
`[skills] delivery`, then `static`. Everything that renders or serves a role uses it:

- `generate --role backend` leaves the served skills out of the static trees and adds the `dynamic-skills` stub;
- `tokens --role backend` does not count served skills in the listing and names them (`served_skills`);
- `ai-rulez mcp --serve-skills --role backend` serves those skills and `find_skill` is scoped to the role;
- `roles resolve`, `roles.json` (`items[].delivery`, `totals.served_skills`, `totals.served_tokens`, `delivery`)
  and `catalog` (`items[].delivery`, `items[].role_delivery`) report it.

`ai-rulez lock` pins the served skills of every role, so `[lock] enforce` covers them.

### Inheritance

`extends` is one level deep: a role that extends a role that itself extends another is an error. The child is
merged over the parent as follows.

| Field | Merge |
| --- | --- |
| `domains` | union, parent first |
| `exclude` lists | union |
| `include` lists | union when both roles set one, otherwise whichever is set |
| `skill_mode`, `delivery` | merged, the child wins per key |
| `description` | the child's, else the parent's |
| `match` | **not** inherited: it identifies who holds *this* role |

`Validate()` fails hard only on a bad name, a duplicate name, an invalid `skill_mode` value or an invalid `delivery` value. Inheritance
problems (unknown parent, cycle, depth greater than one) are logged as warnings there so that
`validate` can report them as [AR972](strict-validation.md). A role with broken inheritance is left out of
`roles.json` with a warning.

Roles are overlayable in `config.local.toml` the same way profiles are: entries merge by `name`, and an entry with
`remove = true` drops a shared role. The local overlay schema includes `roles`.

!!! warning "A role is a content filter, not an access boundary"
    A role narrows the rules, skills, agents, commands and checks that are rendered or served, and sets skill
    modes. It does not narrow hooks, `[permissions]`, `[[mcp_servers]]` or context files: those are rendered for
    every role. Do not use a role to withhold a tool or a credential from someone; the file-based outputs are
    plain files the holder can read and edit.

## Commands

```bash
ai-rulez roles list [--format json]           # every role with item counts and token estimates
ai-rulez roles show <name> [--format json]    # as declared, and with the parent merged in
ai-rulez roles resolve <name> [--format json] # the items the role keeps, with sizes and skill modes

ai-rulez generate --role engineer             # project outputs for the role
ai-rulez generate --user --role engineer      # user-level outputs (~/.claude, ~/.codex, ...)
ai-rulez generate --check --role engineer     # drift check against the role's outputs

ai-rulez tokens --role engineer               # token surface of the role
ai-rulez tokens --by-role                     # one comparison column per role
ai-rulez catalog --format json                # items, owners, versions, roles, lock status
```

`--role` and `--profile` are mutually exclusive, and `--role` cannot be combined with `--plugin` (plugin bundles
are built from the full content). A role replaces profile selection: the content tree is narrowed by the role's
domains and selectors through the same selection path profiles use, so everything downstream in the same run (outputs, the token
report, the usage index) sees the role's slice. Installed skills scoped to profiles are not filtered by
a role, and the machine-local `local/` tree is not narrowed.

## `skill_mode` becomes Claude Code `skillOverrides`

For every skill the role keeps and gives a mode, `generate` writes `skillOverrides.<skill>` in
`.claude/settings.json` (or `~/.claude/settings.json` with `--user`) through the same per-key ownership as
[`[claude.settings.managed]`](settings.md): only the listed skill ids are owned, every skill id the role does not
list is left alone, `clean` takes back exactly what was recorded, and a second run changes nothing. A skill id the
role lists is the role's while the role renders. If you had written a different value for it by hand,
`generate --role` warns, replaces it, and remembers yours in `<config dir>/local/.role-skill-overrides.json`
(machine-local, always gitignored); a plain `generate`, or a role that no longer lists the skill, puts your value
back. This holds for every run that writes the settings: `generate`, each regeneration of `generate --watch --role`,
and `generate --user`; `mcp --serve-skills --role` only serves skills and never writes `.claude/settings.json`, so it
changes nothing there. A value you changed after the role wrote it is yours and is left alone. Switching from one role
to another removes the first role's entries and writes the second's. A role's modes win over the same skill in
`[claude.settings.managed] skill_overrides`.

| Mode | Claude Code behavior |
| --- | --- |
| `on` | listed with its description; the default |
| `name-only` | listed by name only, saving description tokens |
| `user-invocable-only` | hidden from the model; the user can still run it as `/name` |
| `off` | hidden from the model and the user |

## `skill_mode` on other harnesses

Only Claude Code has a per-skill settings key. For the other harnesses ai-rulez renders a mode where the vendor
documents an equivalent and says so where it does not. One content tree is rendered for every preset of the run,
so a mode is written into the skill's `SKILL.md` frontmatter (or, for Codex, `agents/openai.yaml`) and each harness
reads what it understands. A key you set in the skill's own frontmatter is never overwritten.

| Preset | `user-invocable-only` | `off` | `name-only` | Source (read 2026-10-06) |
| --- | --- | --- | --- | --- |
| `claude` | `skillOverrides` | `skillOverrides` | `skillOverrides` | [Claude Code skills](https://code.claude.com/docs/en/skills) |
| `cursor` | `disable-model-invocation: true` | no documented setting | no | [Cursor skills](https://cursor.com/docs/context/skills) |
| `codex` | `agents/openai.yaml` `policy.allow_implicit_invocation: false` | documented (`[[skills.config]] enabled = false`) but needs an absolute path, not rendered | no | [Codex skills](https://developers.openai.com/codex/skills) |
| `copilot` | `disable-model-invocation: true` | `disable-model-invocation: true` and `user-invocable: false` | no | [VS Code agent skills](https://code.visualstudio.com/docs/copilot/customization/agent-skills) |
| `opencode` | no | documented (`permission.skill` = `deny` in `opencode.json`), not rendered | no | [OpenCode skills](https://opencode.ai/docs/skills/) |
| `gemini` | no | only the `/skills disable` command is documented, no settings key | no | [Gemini CLI skills](https://geminicli.com/docs/cli/skills/) |
| every other preset | not documented | not documented | not documented | not checked |

`on` is every harness's default. A cell that says "no" or "not documented" means ai-rulez writes nothing for it and
the skill stays listed on that harness for `user-invocable-only` and `name-only`.

For `off`, a skill is hidden only when every configured preset can hide it. Otherwise `generate --role` follows
`skill_mode_fallback`:

```toml
[role_manifest]
skill_mode_fallback = "drop"   # drop (default) or serve
```

`drop` leaves the skill out of the role's render; `serve` moves it to [served delivery](#delivery), reachable with
`find_skill` on harnesses that can call MCP. The setting lives in `[role_manifest]` because TOML cannot use `roles` as
both an array and a table. Because one tree is rendered, the fallback applies to every harness of the run, Claude Code
included (its `skillOverrides` entry is then not written for a dropped skill). Each skill and harness that is not
honoured is named in a warning. `ai-rulez roles resolve <name>` lists them (`skill_modes` in `--format json`), and
`ai-rulez doctor` reports them.

## Strict validation

`generate --role <name>` prints the AR971 findings of that role as warnings before it writes anything, so a typo in a
domain or in an `exclude` entry is not silent.

| Code | Meaning |
| --- | --- |
| [AR971](strict-validation.md) `role-reference-unknown` | A role lists a domain that does not exist, or a selector / `skill_mode` / `delivery` entry that matches no item (or only matches in a domain the role does not select). |
| [AR972](strict-validation.md) `role-extends-invalid` | Unknown parent, cycle, or inheritance deeper than one level. |
| [AR973](strict-validation.md) `role-unreachable-dependency` | An item the role keeps names a skill in its `skills:` frontmatter that the role drops, or hides from the model with `off` / `user-invocable-only`. Prose references are not analysed. |

## The roles manifest

With `[role_manifest] enabled = true`, `generate` writes `<config dir>/roles.json`. `roles list --format json`
prints the same document. It is deterministic (no timestamps; roles sorted by name, items by kind, domain and id),
so it is safe to commit, and it is versioned by `schema_version`. It is built from the shared sources only: roles
declared in `config.local.toml` and items under `local/` never appear in it, so the committed file is the same on every
machine and `generate --check` does not report it as drifted. Local roles still work with `generate --role` and
`roles list`. The JSON schema is
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
      "domains": ["shared", "backend"],
      "skill_modes": { "review-pr": "name-only" },
      "delivery": { "deploy-*": "served" },
      "items": [
        {
          "kind": "skill", "id": "review-pr", "domain": "shared",
          "path": "domains/shared/skills/review-pr/SKILL.md", "mode": "name-only", "delivery": "static",
          "owner": "platform", "version": "1.2.0", "bytes": 2210, "tokens": 540
        }
      ],
      "totals": { "items": 1, "bytes": 2210, "tokens": 540, "by_kind": { "skill": 1 }, "served_skills": 0, "served_tokens": 0 }
    }
  ]
}
```

`bytes` is the size of the item's source files on disk (a skill counts its resources). `tokens` is an estimate for
the primary file (`SKILL.md`, the rule file, ...) and is an approximation, like every figure of `ai-rulez tokens`.
`delivery` (skills only) is how the skill reaches the role's agent; `served_skills` and `served_tokens` total the
skills that are served only, which cost no listing tokens until `load_skill` runs.
`owner` and `version` come from the item's frontmatter when present.

## Integrating an identity tool or UI

Everything a tool needs is reachable from `--format json` output and versioned documents. ai-rulez never calls the
network, and an integration never needs to parse config files.

1. **Discover the roles.** Read `roles.json` (committed) or run `ai-rulez roles list --format json`. Each role has
   its `name`, its `description`, and `match.groups`, the hints you put there for your tool.
2. **Map a person to a role.** This is your tool's job. For example, an `acli` command can read a person's
   identity-provider groups, find the role whose `match.groups` contains one of them (choosing by your own
   precedence when several match), and print the role name. ai-rulez does not look at `match`.
3. **Preview.** `ai-rulez roles resolve <name> --format json` lists exactly the items the role keeps, with owner,
   version, bytes and tokens. `ai-rulez tokens --role <name> --format json` reports the token surface.
4. **Apply.** Run `ai-rulez generate --user --role <name> --yes` (user level) or `ai-rulez generate --role <name>`
   (project level). Exit code 0 means the files were written.
5. **Show the whole catalog.** `ai-rulez catalog --format json` lists every item with its owner, version, size,
   sha256 digest, the roles that keep it, a summary of each role and the lock status. Its schema is
   [`schema/catalog.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/catalog.v1.schema.json)
   (`--schema-version 2` adds load cost, lint and excerpts; see [Catalog](catalog.md)).
6. **Audit.** [`ai-rulez lock --diff --format json`](lockfile.md) reports what changed between the committed lock
   and the working tree ([`schema/lock-diff.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/lock-diff.schema.json)).

Every JSON document carries `schema_version`; a consumer should refuse a version it does not know. A UI needs no
other interface: lists come from `roles.json` / `catalog`, previews from `roles resolve`, the action is one
`generate` command.

## Roles and the lock file

The [lock file](lockfile.md) pins each role declaration as an item (`kind = "role"`), so adding a role, or
changing what a role includes, shows up in the lock diff and is caught by `lock --check`. The lock pins the
default rendering; a role's rendered outputs are pinned, as one digest per role, when the role sets `pin = true`
(or with `lock --roles`). `lock --check [--role <name>]` and `generate --check --locked --role <name>` then
catch a change in what the role renders that leaves the sources alone, for example a new `skill_mode` entry or a
generator release; see [Composing with roles](lockfile.md#composing-with-roles). Roles without a pin are
unchanged, and `generate --locked --role <name>` still verifies that the *sources* match the lock. Skills a role delivers as `served` are
pinned as `[[served]]` entries (see [Served skills and skill sources](lockfile.md#served-skills-and-skill-sources)),
so `[lock] enforce` holds for a server started with `--role`. Checks are a role-selectable kind and are pinned like
rules.
