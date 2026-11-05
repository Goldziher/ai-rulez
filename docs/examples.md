# Examples

Complete configurations for common setups. Each example shows the files you write in `.ai-rulez/`, the command you
run and what it generates.

Start every example from a project created with `ai-rulez init` (see the [Quick start](quick-start.md)). `init` makes
`.ai-rulez/` an [OKF](okf.md) bundle with `index.md` listings; a tree you build by hand without them is the deprecated
pre-OKF layout and logs a deprecation notice. In an OKF tree, content files use OKF frontmatter: a `type` (`Decision`
for rules, `Concept` for context, `Playbook` for skills), an optional `title`, and ai-rulez settings under
`x-ai-rulez.metadata`. `ai-rulez add` writes this frontmatter for you; `ai-rulez generate` keeps the `index.md`
listings up to date.

## Rules scoped to part of the repository

A web project where TypeScript rules should load only when the assistant works on TypeScript files.

`.ai-rulez/config.toml`:

```toml
version = "5.0"
name = "web"
presets = ["cursor", "copilot", "codex"]

[rules]
scoped_fallback = "skill"
```

`.ai-rulez/rules/typescript.md`:

```markdown
---
type: Decision
title: TypeScript
x-ai-rulez:
  kind: rule
  id: typescript
  metadata:
    priority: high
    paths: ["src/**/*.ts", "src/**/*.tsx"]
---

# TypeScript

- Enable `strict` in `tsconfig.json`.
- Prefer `unknown` over `any`.
```

```bash
ai-rulez generate
```

Each tool with a rules folder gets the rule there, in its own scoping syntax. Codex has no rules folder, so
`scoped_fallback = "skill"` serves the rule as a skill that applies to the same paths:

```text
.cursor/rules/typescript.mdc                      # globs: src/**/*.ts,src/**/*.tsx
.github/instructions/typescript.instructions.md   # applyTo: src/**/*.ts,src/**/*.tsx
.agents/skills/rule-typescript/SKILL.md           # paths: [src/**/*.ts, src/**/*.tsx]
```

The rule is not in `AGENTS.md`, so it costs nothing while the assistant works on other files. Without
`scoped_fallback = "skill"`, `generate` inlines scoped rules into `AGENTS.md` for tools without a rules folder and
warns that the tools with one then load them in every session; see
[Glob-scoped rules and AGENTS.md](rules.md#glob-scoped-rules-and-agentsmd).

## A skill with a script

Skills are loaded on demand: the assistant sees the `description` and reads the body only when the task matches.
Files next to `SKILL.md` (`scripts/`, `references/`, `assets/`) are copied with it.

```text
.ai-rulez/skills/release/
├── SKILL.md
└── scripts/
    └── check.sh
```

`.ai-rulez/skills/release/SKILL.md`:

```markdown
---
type: Playbook
title: Release
description: Cut a release of the web app. Use when asked to tag, version or publish a release.
x-ai-rulez:
  kind: skill
  id: release
---

# Release

1. Run `scripts/check.sh` and stop if it fails.
2. Bump the version in `package.json`.
3. Tag the commit `vX.Y.Z` and push the tag.
```

With the presets of the previous example, `ai-rulez generate` writes the skill, script included, to the shared skill
folder:

```text
.agents/skills/release/SKILL.md
.agents/skills/release/scripts/check.sh
```

Tools with their own skill folder get a copy there too, for example `.claude/skills/release/` for `claude`.

See [Skill frontmatter](skills.md) for `allowed-tools`, served skills and other keys.

## One instruction file per tool, and an MCP server

By default all tools share one `AGENTS.md`. With `agents_md = false`, each tool gets its own root file. This example
also limits one rule to a single tool and adds an MCP server for every tool.

`.ai-rulez/config.toml`:

```toml
version = "5.0"
name = "billing"
agents_md = false
presets = ["claude", "gemini", "copilot"]

[[mcp_servers]]
name = "postgres"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres@0.6.2", "${DATABASE_URL}"]
```

`.ai-rulez/rules/money.md`:

```markdown
---
type: Decision
title: Money
x-ai-rulez:
  kind: rule
  id: money
  metadata:
    priority: critical
---

# Money

- Store amounts as integer minor units, never floats.
```

`.ai-rulez/rules/claude-subagents.md`, written only for Claude Code:

```markdown
---
type: Decision
title: Subagents
x-ai-rulez:
  kind: rule
  id: claude-subagents
  metadata:
    targets: ["claude"]
---

# Subagents

- Run database migrations in a subagent, never in the main session.
```

```bash
ai-rulez generate
```

```text
CLAUDE.md
.claude/rules/money.md
.claude/rules/claude-subagents.md
GEMINI.md
.gemini/settings.json                       # postgres and ai-rulez MCP servers
.github/copilot-instructions.md
.github/instructions/money.instructions.md
.mcp.json                                   # postgres and ai-rulez MCP servers
.vscode/mcp.json
```

`${DATABASE_URL}` is written as a reference, never as the resolved value, in tools that support environment
references. `generate` also adds the ai-rulez MCP server itself; set `[mcp] self_server = false` to leave it out. See
[Configuration](configuration.md) for MCP server keys and [Rules](rules.md#targets) for `targets`.

## Teams with domains and profiles

A platform repository where the backend and frontend teams each have their own rules. Root content applies to
everyone; a profile selects which domains are added.

```text
.ai-rulez/
├── config.toml
├── rules/security.md
└── domains/
    ├── backend/rules/database.md
    └── frontend/rules/components.md
```

Create the domains and their rules with the CLI:

```bash
ai-rulez domain add backend
ai-rulez domain add frontend
ai-rulez add rule security --priority critical --content '# Security

- Validate all user input.'
ai-rulez add rule database --domain backend --priority critical --content '# Database

- Use prepared statements.'
ai-rulez add rule components --domain frontend --content '# Components

- One component per file.'
```

Then define the profiles in `.ai-rulez/config.toml`:

```toml
version = "5.0"
name = "platform"
presets = ["claude", "cursor", "codex"]
default = "full"

[profiles]
full = ["backend", "frontend"]
backend = ["backend"]
frontend = ["frontend"]
```

```bash
ai-rulez generate                     # default profile "full": security, database, components
ai-rulez generate --profile backend   # security, database
ai-rulez generate --profile frontend  # security, components
```

Set `default` when you define `[profiles]`. Without it, a run without `--profile` includes no domain content and
`generate` warns. See [Domains & profiles](domains.md) and, for per-person filtering, [Roles](roles.md).

## Shared rules from another repository

Keep organization-wide rules in one repository and include them everywhere. A version constraint picks the newest
matching tag.

```toml
version = "5.0"
name = "web"
presets = ["cursor", "codex"]

[[includes]]
name = "org-standards"
source = "https://github.com/acme/ai-standards.git"
version = "^1.2"
include = ["rules", "skills"]
merge_strategy = "local-override"
```

`include` limits what is taken from the source; `merge_strategy = "local-override"` (the default) lets a local item
with the same name win. Pin what was resolved in `.ai-rulez/ai-rulez.lock` and commit it with the configuration:

```bash
ai-rulez generate
ai-rulez lock
git add .ai-rulez
```

`ai-rulez lock --outdated` reports newer matching tags and `ai-rulez update` moves the pin. See [Includes](includes.md)
and [Lock file](lockfile.md).

## Monorepo with nested projects

Each service keeps its own `.ai-rulez/` next to its code, and the repository root has one for shared conventions.

```text
acme/
├── .ai-rulez/
│   ├── config.toml
│   └── rules/commits.md
└── services/
    └── api/
        └── .ai-rulez/
            ├── config.toml
            └── rules/endpoints.md
```

```bash
ai-rulez generate --recursive
ai-rulez validate --recursive
```

Each root generates its own files (`AGENTS.md`, `CLAUDE.md`, ...) in its own directory. Tools that read nested
instruction files load the service's file in addition to the root's when they work in `services/api/`. A local
include in a nested project must stay inside that project's directory; share content across services with a git
include or the repository-root configuration. See [Monorepo](monorepo.md).
