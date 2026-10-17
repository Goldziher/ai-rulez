# Monorepos and Large Projects

For large projects with multiple teams, organize your `.ai-rulez/` configuration using domains and profiles to provide relevant context while avoiding overwhelming AI assistants with irrelevant information.

This guide outlines best practices for managing AI context at scale.

<p align="center">
  <img src="assets/monorepo.gif" alt="ai-rulez generate --recursive over two nested roots, recursive validation, and a token budget check" width="820" />
</p>

---

## The Core Strategy: Domain-Based Organization

The most effective strategy for large projects is to organize related rules, context, and skills into domains. Each domain represents a team, service, or feature area.

!!! success "The Goal: High-Relevance, Low-Token Context"

    By organizing into domains and using profiles, you provide your AI assistant with only the context it needs for the task at hand. A developer working on the frontend gets frontend-specific rules and context, while the backend developer gets API patterns and database standards. This results in faster, more accurate, and more relevant AI responses.

!!! info "Domains and Profiles"
    - **Domains** organize content by team, service, or feature (`backend`, `frontend`, `mobile`)
    - **Profiles** select which domains are included in generation (`full`, `backend-team`, etc.)

A typical layout might look like this:

```text
my-project/
└── .ai-rulez/
    ├── config.toml              # ⬅️ Main config: presets, profiles, domains
    ├── rules/                   # ⬅️ Shared rules (all teams)
    ├── context/                 # ⬅️ Shared context (all teams)
    ├── skills/                  # ⬅️ Shared skills (all teams)
    └── domains/
        ├── backend/             # ⬅️ Backend team content
        │   ├── rules/
        │   ├── context/
        │   └── skills/
        ├── frontend/            # ⬅️ Frontend team content
        │   ├── rules/
        │   ├── context/
        │   └── skills/
        └── qa/                  # ⬅️ QA team content
            └── rules/
```

When you run `ai-rulez generate --profile backend`, it includes root content plus backend-specific content.

---

## Best Practice: Root Configuration

Your root `.ai-rulez/` should contain high-level, cross-cutting concerns that apply to all teams.

!!! tip "What to put in Root Content"

    - **System Architecture:** High-level overview of the tech stack and service interactions
    - **Cross-Cutting Concerns:** Rules that apply everywhere (security, logging, error handling)
    - **General Standards:** Code quality, testing, git workflow, deployment processes
    - **Base Skills:** Shared AI skills used by all teams

**`.ai-rulez/config.toml`:**

```toml
version = "5.0"
name = "My Full-Stack Project"
description = "Microservices project with React frontend and Go backend"

presets = ["claude", "cursor", "gemini"]
default = "full"
gitignore = true

[profiles]
full = ["backend", "frontend", "qa"]
backend = ["backend", "qa"]
frontend = ["frontend", "qa"]
qa = ["qa"]
```

**`.ai-rulez/context/architecture.md`:**

```markdown
# System Architecture

This is a microservices project with:

- Go backend services
- React web frontend
- PostgreSQL databases
- Kubernetes orchestration
```

**`.ai-rulez/rules/security.md`:**

```markdown
---
priority: critical
---

# Security Standards

- All secrets use environment variables
- Validate all user input
- Use HTTPS for external APIs
```

**`.ai-rulez/rules/git-workflow.md`:**

```markdown
---
priority: high
---

# Git Workflow

- Feature branches from main
- Squash commits before merge
- All PRs require code review
```

## Best Practice: Domain-Specific Content

Domain directories contain specific, detailed context for that team or service.

!!! tip "What to put in Domain Content"

    - **Technology Patterns:** Concrete examples for frameworks and libraries
    - **Domain Logic:** Business rules specific to that service or feature
    - **Domain Skills:** Specialized AI prompts for that team's expertise

**`.ai-rulez/domains/backend/rules/database.md`:**

```markdown
---
priority: critical
---

# Database Standards

- Use prepared statements to prevent SQL injection
- Always add migrations for schema changes
- Index foreign keys
```

**`.ai-rulez/domains/backend/rules/api-design.md`:**

```markdown
---
priority: high
---

# API Design

- Follow RESTful principles
- Use consistent error responses
- Version APIs from the start
```

**`.ai-rulez/domains/backend/context/architecture.md`:**

```markdown
# Backend Architecture

## Services

- API Gateway (Go, port 8000)
- User Service (Go, port 8001)
- Product Service (Go, port 8002)
- Order Service (Go, port 8003)

## Database

- PostgreSQL 14+
- Replication enabled
- Automated daily backups
```

**`.ai-rulez/domains/backend/skills/database-expert/SKILL.md`:**

```markdown
---
priority: high
description: "Database design and optimization specialist"
---

# Database Expert

You are an expert in PostgreSQL with knowledge of:

- Schema design and normalization
- Query optimization
- Performance tuning
```

**`.ai-rulez/domains/frontend/rules/components.md`:**

```markdown
---
priority: high
---

# Component Guidelines

- One component per file
- Use TypeScript for type safety
- Write unit tests for all components
- Use composition over inheritance
```

**`.ai-rulez/domains/frontend/context/design-system.md`:**

```markdown
# Design System

## Color Palette

- Primary: #3B82F6
- Secondary: #8B5CF6
- Neutral: #6B7280

## Typography

- Headings: Inter Bold
- Body: Inter Regular
```

## Best Practice: Profile Selection

Design profiles to match your team structure and workflows.

```toml
[profiles]
# Full platform: all content
full = ["backend", "frontend", "qa", "devops"]

# Team-specific
backend-team = ["backend", "qa"]
frontend-team = ["frontend", "qa"]
qa-team = ["qa"]
devops-team = ["devops"]

# Full-stack developers
full-stack = ["backend", "frontend", "qa"]

# CI/QA environment
ci-cd = ["backend", "frontend", "qa", "devops"]
```

## Best Practice: For Monorepos

If using multiple `.ai-rulez/` directories in a monorepo, use the `--recursive` flag:

```bash
# Process all .ai-rulez/ directories recursively
ai-rulez generate --recursive
```

### Speed on large repositories

`generate --recursive`, `generate --check --recursive` and `validate --recursive` process the roots concurrently, on at most one worker per CPU (`GOMAXPROCS`; `validate --recursive --external` checks one root at a time, because the scanners share the terminal and one baseline file). `generate --check --recursive` and `validate --recursive` report results in path order whatever the finishing order, so the output and the exit code are the same on every run. Each root prints a `[3/12] svc/.ai-rulez/config.toml (1.2s)` line on stderr as it is reported; `--quiet` and `--format json` silence it. In those two commands each root collects the warnings its work raises (refused symlinked content, `--gitignore` notices, policy warnings, configuration-load log lines) and the run prints them with that root's result, so their order is the root order on every run. `generate --recursive`, which writes files, shows a progress bar instead of the `[n/N]` lines, and a root's load warnings (for example the legacy layout notice) are logged as they happen, so they can interleave between roots.

A read-only recursive check (`generate --check`, `validate`) also shares one listing of the git index between the roots of a repository, instead of asking git whether each root's config is tracked once per root. The cost of a git call grows with the number of tracked files, so this matters most in repositories with tens of thousands of them. Pass `--debug` to see where time goes; `ai-rulez doctor --debug` logs the duration of each check.

**Root configuration** (`/.ai-rulez/config.toml`):

```toml
version = "5.0"
name = "Monorepo Platform"

presets = ["claude", "cursor"]
default = "full"

[profiles]
full = ["shared"]
```

**Service-specific** (`/backend/.ai-rulez/config.toml`):

```toml
version = "5.0"
name = "Backend Service"

presets = ["claude"]
default = "backend"

[profiles]
backend = ["api", "database"]
```

By combining domain organization with thoughtful profile design, you can create scalable, maintainable configurations that grow with your project.

### The shared `.gitignore` block

With `gitignore = true`, a recursive run writes one managed block at the git toplevel holding every root's patterns
(`svc/CLAUDE.md`, `svc/.claude/skills/`, ...), and `generate --recursive --check` verifies that block once after the
last root. The root's own entries are unanchored (`AGENTS.md`, `CLAUDE.md`), so they also match the file of every nested
root. To commit generated files instead, list them in each root's [`gitignore_exclude`](configuration.md#gitignore_exclude);
a root applies its list to its own outputs, so a project that commits everything repeats
`gitignore_exclude = ["**/AGENTS.md", "**/CLAUDE.md", ".claude/"]` in each config. When the block hides a file git
tracks (a root-anchored `!.claude/rules/*.md` of yours does not reach a nested root's `svc/.claude/rules/x.md`),
`generate` warns once for the whole recursive block and names the nested path.


---

## Nested Instructions Over MCP

A repository can keep `AGENTS.md`, `CLAUDE.md`, `.claude/`, `.agents/` or `skills/` in any package, and a harness
loads them only once it works there. A planner at the root, or an agent changing several packages, can ask the MCP
server what applies to a path: `ai-rulez mcp --serve-skills --discover-skills` indexes the repository by scope, and
`find_skill`, `find_context` and `load_context` take a `path` or a `scope`. The nearest scope wins a skill name, and the
index reads the files that already exist, so it needs no `generate` run. See
[Monorepos: scopes and nested context](mcp-server.md#monorepos-scopes-and-nested-context).

On a large repository the index is kept between runs: summaries of the indexed files and the scan verdicts of clean
content live in the user cache (`[mcp] index_cache = "user" | "repo" | "off"`), so a warm start takes about a tenth of a
cold one, and `ai-rulez cache status` / `ai-rulez cache clear` show and drop it
([The index cache](mcp-server.md#the-index-cache)). While the server runs, a file watcher refreshes the index when a
nested `AGENTS.md` or `.claude/skills` directory appears, changes or goes, and the server sends
`resources/list_changed` ([Live refresh](mcp-server.md#live-refresh-of-the-index)). An agent can also add a skill where
a harness will load it, with `create_skill` and `scope`, behind `allow_write`
([Writing a skill into a scope](mcp-server.md#writing-a-skill-into-a-scope)).

---

## Scoped Rule Files

For `[[scopes]]` (see [Configuration](configuration.md#scopes)), root files such as `CLAUDE.md` stay in the scope directory, but native rule files are written to the **root** rules folder. Rules-folder nesting is not uniform across presets: Copilot reads its instructions at the repository root only, while Claude Code discovers `.claude/rules/*.md` recursively — a nested `<dir>/.claude/rules/` rule loads, with its `paths` resolved relative to `<dir>` — and Antigravity reads `.agents/rules/` at any directory level. Nesting in the other presets is not verified. Writing every scope's rules to the root folder gives one placement that every preset with a rules folder reads, and the scope path as a qualifier and glob prefix keeps file names unique and each rule limited to the scope's files:

| Preset | Scoped rule file for `packages/api`, rule `style` |
| --- | --- |
| Claude | `.claude/rules/packages-api/style.md` |
| Cursor | `.cursor/rules/packages-api/style.mdc` |
| Copilot | `.github/instructions/packages-api/style.instructions.md` |
| Devin, Cline, Junie | `<rules dir>/packages-api--style.md` |
| Antigravity | `.agents/rules/packages-api--style.md` |

Globs in a scope's rules are relative to the scope root: a rule with `paths: ["**/*.go"]` gets `packages/api/**/*.go`, and a rule without globs applies to `packages/api/**`. A rule with only negated globs also gets `packages/api/**`. A glob that climbs out of the scope with `..` (also inside braces) skips that rule for the scope with a warning. `auto` and `manual` rules keep their mode. With `[rules] mode = "inline"` only path-scoped items move to the root folder; everything else stays in the scope's root file. Two files that map to the same path (names compare case-insensitively) do not fail: the source that sorts later is written as `<id>-<6 hex>` with a warning (see [File names](rules.md#file-names)). Generation fails only when two scopes produce the same qualifier (for example `packages/api` and `packages-api`); rename one of the scope paths.

Emitting a scope's `claude` rules into the scope's own `<dir>/.claude/rules/` instead of the root folder is a possible future change; today the root placement is uniform, because not every preset reads a nested rules folder.

Limitations:

- Root files left in a scope (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, ...) are read by tools that load nested files. Copilot (`.github/copilot-instructions.md`) reads its instructions at the repository root only, so rules or context that stay inline in a scope's copy are never loaded; generation warns and names them. Use path-scoped rules or `[rules] mode = "split"` for that preset.
- Domains the root run already renders are not repeated in a scope.
- Presets without split rule files (custom provider specs with a non-split `outputs.rules`) keep a scope's rules in the scope directory; generation warns.
