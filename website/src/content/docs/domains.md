---
title: "Domains and profiles"
description: "A domain is a named folder of content (.ai-rulez/domains/<name>/) for one team, service or area of the project."
---

A **domain** is a named folder of content (`.ai-rulez/domains/<name>/`) for one team, service or area of the
project. A **profile** is a named list of domains. `generate --profile <name>` renders the root content plus the
domains the profile names, so the backend team's assistants get the backend rules and not the frontend ones.

## Example

```text
.ai-rulez/
├── config.toml
├── rules/
│   └── commit-messages.md          root: every profile gets it
└── domains/
    ├── backend/
    │   └── rules/
    │       └── database.md
    └── frontend/
        └── rules/
            └── components.md       paths: ["web/**/*.tsx"]
```

```toml
# .ai-rulez/config.toml
presets = ["claude", "cursor", "gemini"]
default = "backend"

[profiles]
backend = ["backend"]
frontend = ["frontend"]
```

`ai-rulez init --domains backend,frontend` creates the folders, and `ai-rulez add rule` writes the rules:

```bash
ai-rulez add rule commit-messages --content "Use Conventional Commits."
ai-rulez add rule database --domain backend --priority critical \
  --content "Use prepared statements. Every schema change ships a migration."
```

```markdown
<!-- .ai-rulez/domains/backend/rules/database.md -->
---
type: Decision
title: Database
x-ai-rulez:
  kind: rule
  id: database
  domain: backend
  metadata:
    priority: critical
---

Use prepared statements. Every schema change ships a migration.
```

```markdown
<!-- .ai-rulez/domains/frontend/rules/components.md -->
---
type: Decision
title: Components
x-ai-rulez:
  kind: rule
  id: components
  domain: frontend
  metadata:
    paths: ["web/**/*.tsx"]
---

One component per file.
```

`add rule` writes `priority: medium` when you pass no `--priority`, which is why the root rule below shows a priority
line; a rule without an explicit `priority` has none.

`ai-rulez generate` uses the `default` profile, `backend`. The shared `AGENTS.md`, read by all three tools, holds the
root rule and the backend rule:

```markdown
<!-- AGENTS.md -->
...
# acme

## Rules

### database

**Priority:** critical

Use prepared statements. Every schema change ships a migration.

### commit-messages

**Priority:** medium

Use Conventional Commits.
```

`ai-rulez generate --profile frontend` replaces the backend rule with the frontend one. The scoped rule gets
`.claude/rules/components.md` and `.cursor/rules/components.mdc`; files of the previous profile are removed. Because
Gemini has no rules folder, `AGENTS.md` also inlines the scoped rule and `generate` warns; see
[Glob-scoped rules and AGENTS.md](/ai-rulez/rules/#glob-scoped-rules-and-agentsmd).

## Domain folders

A domain folder has the same layout as the root of `.ai-rulez/`:

```text
.ai-rulez/domains/<name>/
├── rules/
├── context/
├── skills/<id>/SKILL.md
├── agents/
├── commands/
└── checks/
```

Any subset of these folders makes a domain. `ai-rulez init --domains a,b` creates them; `add ... --domain <name>` fails
until the domain folder exists. Domain names are folder names; use names that say who owns the content
(`backend`, `payments`, `mobile`), not `misc` or `shared`. Content everybody needs belongs at the root.

## What a profile selects

A run renders:

- all root content;
- the domains the profile names;
- globally enabled [builtin](/ai-rulez/configuration/) packs and every domain that comes from an [include](/ai-rulez/includes/),
  whatever the profile.

Profiles also select [installed skills](/ai-rulez/installed-skills/) and MCP servers that list them in their `profiles`
field.

| Setting                 | Meaning                                                                                       |
| ----------------------- | --------------------------------------------------------------------------------------------- |
| `[profiles] <name> = [...]` | the domains the profile selects                                                            |
| `default`               | the profile `generate` uses without `--profile`                                               |
| `--profile <name>`      | select a profile for one run (`generate`, `tokens`, `export okf`, ...)                         |

With no profile selected and no `default` set, the built-in `default` profile applies: every domain when no
`[profiles]` are defined, otherwise only root content plus builtin and include-sourced domains. So once you add
`[profiles]` without a `default`, a plain `generate` drops all domain content and removes its outputs; `generate`
warns about this. Set a top-level `default = "<profile>"`, or define a profile named `default` (for example
`default = ["*"]` under `[profiles]`).

### Domain list entries

| Entry            | Meaning                                                                                             |
| ---------------- | --------------------------------------------------------------------------------------------------- |
| `"backend"`      | the domain `backend`                                                                                |
| `"*"`            | every domain the configuration knows                                                                |
| `"!frontend"`    | remove `frontend` from what the other entries selected                                              |
| `"builtin:docker"` | the builtin pack `docker`, for this profile only; `builtin:<name>/<item>` selects one item of a pack |

```toml
[profiles]
backend = ["backend", "builtin:docker"]
everything-but-mobile = ["*", "!mobile"]
```

- A list made only of `!` entries is an error: there is nothing to subtract from. Write `"*"` first.
- `builtin:<name>` works even when the root `builtins` setting is absent or `false`. A pack the root `builtins`
  setting enables stays global wherever else it is named.
- An unknown domain name selects nothing; loading the configuration logs a warning naming it.

## Composing profiles

Select several profiles at once by separating their names with commas. The result is the union of their domains, in
the order they are first named:

```bash
ai-rulez generate --profile base,backend
ai-rulez tokens --profile base,backend
```

One profile can then hold what everybody installs, while the others add only their extras:

```toml
[profiles]
base = ["conventions", "security"]
backend = ["api", "database"]
frontend = ["web"]
```

`--profile base,backend` and `--profile base,frontend` cover both teams without a `base-backend` and `base-frontend`
profile each. A composed value works anywhere a profile name does, including `default` and a scope's `profile`:

```toml
default = "base,backend"
```

- Every element must be a defined profile. An unknown one is an error naming that element.
- Profile lists name domains, never other profiles, so composition is one level deep.
- The built-in `default` composes only if you defined a profile named `default`.
- Whitespace and empty elements are ignored: `base, backend` and `base,backend,` are the same.
- A profile name may not contain a comma.

## Name collisions

When an item of the same kind and name exists in more than one place, one copy is kept and the others are dropped
with a warning naming both files:

```text
WARN  Duplicate rule collapsed name=database kept=.ai-rulez/rules/database.md dropped=.ai-rulez/domains/backend/rules/database.md
```

Precedence is root, then on-disk domain, then include-sourced domain, then builtin. It applies to rules, context,
skills, agents, commands and checks alike; see [Deduplication by name](/ai-rulez/configuration/#deduplication-by-name). To
keep a rule specific to one domain, give it a name no other layer uses.

## Domain settings

`[domains.<name>]` holds settings for one domain's content:

```toml
[domains.runbooks]
delivery = "served"    # serve this domain's skills over MCP instead of writing them
```

| Key        | Type   | Default  | Meaning                                                                         |
| ---------- | ------ | -------- | ------------------------------------------------------------------------------- |
| `delivery` | string | `static` | default [delivery](/ai-rulez/skills/#dynamic-loading-delivery) of the domain's skills   |

## Profiles and roles

A profile answers "what does this repository ship". When the question is "what does a backend engineer, an on-call
responder or a support agent get on their machine", use [roles](/ai-rulez/roles/): they add per-kind include and exclude
selectors, one level of inheritance and per-skill modes on top of a domain list, and are selected with
`generate --role`. See [Profiles vs roles](/ai-rulez/roles/#profiles-vs-roles).

## Moving to domains

To split a flat `.ai-rulez/rules/` into domains:

```bash
mkdir -p .ai-rulez/domains/backend/rules .ai-rulez/domains/frontend/rules
git mv .ai-rulez/rules/backend-api.md .ai-rulez/domains/backend/rules/api.md
git mv .ai-rulez/rules/frontend-components.md .ai-rulez/domains/frontend/rules/components.md
```

Then add the `[profiles]`, and check the result:

```bash
ai-rulez validate
ai-rulez generate --profile backend
```

In an [OKF tree](/ai-rulez/okf/#the-native-tree), `generate` also refreshes the `index.md` listings of the folders you
changed.

## Troubleshooting

| Symptom                              | Cause and fix                                                                                     |
| ------------------------------------ | ------------------------------------------------------------------------------------------------- |
| Domain content is missing            | The domain is not in the active profile. Check `default` and `[profiles]`, or pass `--profile`.   |
| `profile not found: x`               | The name, or one element of a composed value, is not defined. The error lists the available ones.  |
| A domain rule never appears          | A root item with the same name wins. `validate` lists every collapsed duplicate.                  |
| Files of another profile remain      | They are removed by the next `generate`; hand-written files in rules folders are never touched.    |
