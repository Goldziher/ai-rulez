# Domains and Profiles

Organize AI configuration by team, subsystem, or project area.

## What Are Domains?

Domains are named areas of your project with their own rules, context, and skills. Use them for:

- Multi-team projects (backend, frontend, mobile teams)
- Multi-service architectures (API, database, cache, queue)
- Feature areas (auth, payments, search)
- Environments (dev, staging, production)

## What Are Profiles?

Profiles specify which domains are included when generating. Example:

```toml
[profiles]
full = ["backend", "frontend", "qa"]   # All teams
backend = ["backend", "qa"]            # Backend team only
frontend = ["frontend", "qa"]          # Frontend team only
qa = ["qa"]                            # QA team only
```

Generation always includes:

- All root content (rules, context, skills, agents, commands)
- Content from the selected domains only
- Globally-active builtins and include-sourced domains, whatever the profile

## Directory Structure

```text
.ai-rulez/
├── config.toml              # Main config with presets and profiles
├── rules/                   # Base rules (all teams get these)
├── context/                 # Base context (all teams get these)
├── skills/                  # Base skills (all teams get these)
└── domains/
    ├── backend/
    │   ├── rules/
    │   │   ├── api-design.md
    │   │   └── database.md
    │   ├── context/
    │   │   └── backend-architecture.md
    │   └── skills/
    │       └── database-expert/
    │           └── SKILL.md
    ├── frontend/
    │   ├── rules/
    │   │   ├── component-guidelines.md
    │   │   └── accessibility.md
    │   ├── context/
    │   │   └── design-system.md
    │   └── skills/
    │       └── ux-expert/
    │           └── SKILL.md
    └── qa/
        └── rules/
            └── testing-strategy.md
```

## Common Organization Patterns

### Service-Based Domains

For microservices or service-oriented architecture:

Domains: `api` (REST API service), `database` (database layer), `cache` (caching layer),
`queue` (message queue), `frontend` (web UI).

**Profiles:**

```toml
[profiles]
full = ["api", "database", "cache", "queue", "frontend"]
backend = ["api", "database", "cache", "queue"]
frontend = ["frontend"]
infrastructure = ["database", "cache", "queue"]
```

### Team-Based Domains

For organizations with dedicated teams:

Domains: `backend` (Go microservices), `frontend` (React web app), `mobile` (React Native),
`qa` (testing and quality assurance), `devops` (infrastructure and deployment).

**Profiles:**

```toml
[profiles]
full = ["backend", "frontend", "mobile", "qa", "devops"]
backend-team = ["backend", "qa"]
frontend-team = ["frontend", "qa"]
mobile-team = ["mobile", "qa"]
qa-team = ["qa"]
devops-team = ["devops"]
ci-all = ["backend", "frontend", "mobile", "qa", "devops"]
```

### Feature-Based Domains

For projects organized by feature:

Domains: `auth` (authentication and authorization), `payments` (payment processing),
`notifications` (email, SMS, push), `search` (search and indexing), `analytics` (data collection).

**Profiles:**

```toml
[profiles]
full = ["auth", "payments", "notifications", "search", "analytics"]
backend = ["auth", "payments", "notifications", "search", "analytics"]
frontend = ["notifications", "search"]
```

### Environment-Based Domains

For different rules per environment:

Domains: `dev` (development guidelines), `staging` (staging constraints), `prod`
(production rules), `security-hardened` (extra security measures).

**Profiles:**

```toml
[profiles]
development = ["dev"]
staging = ["staging", "security-hardened"]
production = ["prod", "security-hardened"]
```

## Domain Names

### Good Names

Use names that indicate ownership or responsibility:

- `backend`, `frontend`, `mobile` (service boundaries)
- `api`, `database`, `cache` (technical components)
- `auth`, `payments`, `search` (feature areas)
- `dev`, `staging`, `prod` (environments)
- `golang`, `typescript`, `python` (technology)

### Avoid

- `team1`, `team2` (not descriptive)
- `a`, `b`, `c` (unclear)
- `everything`, `shared`, `misc` (too broad or ambiguous)

## Creating a Domain

### Step 1: Create the directory structure

```bash
mkdir -p .ai-rulez/domains/backend/{rules,context,skills,agents}
mkdir -p .ai-rulez/domains/frontend/{rules,context,skills,agents}
```

### Step 2: Add content to the domain

**`domains/backend/rules/database.md`:**

```markdown
---
priority: critical
---

# Database Standards

- Use prepared statements to prevent SQL injection
- Always add database migrations
- Index foreign keys for performance
- Document schema changes
```

**`domains/backend/context/architecture.md`:**

```markdown
# Backend Architecture

## Services

- API Gateway (Go)
- User Service (Go)
- Product Service (Go)
- Order Service (Go)

## Database

- PostgreSQL 14+
- Replication enabled
- Automated backups
```

### Step 3: Update config.toml

```toml
version = "5.0"
name = "my-platform"

presets = ["claude", "cursor"]
default = "full"

[profiles]
full = ["backend", "frontend"]
backend = ["backend"]
frontend = ["frontend"]
```

### Step 4: Generate and test

```bash
# Generate for all domains
ai-rulez generate --profile full

# Generate for backend only
ai-rulez generate --profile backend

# Generate for frontend only
ai-rulez generate --profile frontend
```

## Using Domains in Your Workflow

### Single Domain Per Team

Backend team only needs backend rules:

```bash
# Backend team runs
ai-rulez generate --profile backend
# Gets: root content + backend content
```

Frontend team only needs frontend rules:

```bash
# Frontend team runs
ai-rulez generate --profile frontend
# Gets: root content + frontend content
```

### Multiple Domains Per Person

If one person works on multiple areas:

```toml
[profiles]
full-stack = ["backend", "frontend"]
```

```bash
ai-rulez generate --profile full-stack
# Gets: root + backend + frontend content
```

### Builtin Packs Per Profile

A builtin pack can be scoped to one profile by referencing it as `builtin:<name>` in that
profile's domain list:

```toml
[profiles]
backend = ["backend", "builtin:docker"]
frontend = ["frontend"]
```

The `backend` profile gets the `docker` builtin; `frontend` does not. This differs from the
root `builtins` field, which makes a pack global once it is enabled. The reference is an
explicit opt-in, so it works even when `builtins` is absent or `false`, and the `builtin:`
prefix keeps the pack from colliding with a local domain of the same name. A pack the root
field already enabled stays global regardless of where else it is named.

## Domain Content Priority

When the same name exists in both root and a domain:

```text
.ai-rulez/rules/testing.md              (root)
.ai-rulez/domains/backend/rules/testing.md   (domain)
```

The **root version takes precedence** and the domain copy is dropped, with a
warning naming both sources. Precedence runs root > on-disk domain >
include-sourced domain > builtin, and applies to rules, context, skills, agents
and commands alike — see
[Deduplication by Name](configuration.md#deduplication-by-name).

```text
backend profile gets:
  - everything from .ai-rulez/rules/*, including the root testing.md
  - every other backend rule from .ai-rulez/domains/backend/rules/

frontend profile gets:
  - everything from .ai-rulez/rules/* (including testing.md)
  - frontend-specific rules from domains/frontend/
```

To make a rule domain-specific, give it a name no other layer uses, or remove the
root copy.

## Composing Profiles

Several profiles can be selected at once by separating their names with commas. The
result is the union of their domains, de-duplicated, in the order the domains are first
named:

```bash
ai-rulez generate --profile base,backend
ai-rulez tokens --profile base,backend
```

This is what lets one profile hold the content everybody installs while the others add
only their own extras:

```toml
[profiles]
base = ["conventions", "security"]
backend = ["api", "database"]
frontend = ["web"]
```

`--profile base,backend` and `--profile base,frontend` then cover both roles without a
hand-written `base-backend` and `base-frontend` profile each — the combinatorial set a
third role would double.

A composed value works anywhere a profile name does, including the config's own default
and a scope's `profile`:

```toml
default = "base,backend"
```

Rules:

- **Every element must be a defined profile.** An unknown one is an error naming the
  element that was wrong, not the whole value.
- **Profile values name domains, never other profiles.** Composition is one level deep,
  so there is no nesting and no cycle to worry about.
- **`default` composes only if you defined it.** The built-in `default` profile is a
  fallback rule rather than a domain list, so it is meaningful only on its own.
- **Whitespace and empty elements are ignored**: `base, backend` and `base,backend,` both
  select the same two profiles. A value that is nothing but separators selects no profile
  and is reported as not found.
- **A profile name may not contain a comma**, since it could then never be selected.

## Roles

When the question is "what should a backend engineer or a support agent get on their machine" rather than "what
does this repository ship", use [Roles](roles.md): they add per-kind `include` / `exclude` selectors, one level
of inheritance and a per-skill `skill_mode` on top of a domain list, and are selected with `generate --role`.

## Advanced Profile Combinations

### Multi-Level Profiles

Profiles can include multiple domains with shared subsets:

```toml
[profiles]
# Full stack for dev team
full-dev = ["backend", "frontend", "devops", "qa"]

# Minimal for contractors
frontend-only = ["frontend"]

# Security-focused for compliance
security-audit = ["security", "backend", "database"]

# Performance optimization
perf-team = ["backend", "database", "cache"]
```

### Environment-Specific Profiles

```toml
[profiles]
# Development: loose constraints
dev = ["dev-guidelines", "logging-verbose"]

# Staging: stricter
staging = ["staging-checks", "logging-standard", "security-checks"]

# Production: strictest
production = ["production-critical", "logging-minimal", "security-hardened", "compliance"]
```

### Feature-Based Selection

```toml
[profiles]
# New features team
features = ["feature-auth", "feature-payments", "feature-notifications"]

# Infrastructure team
infrastructure = ["database", "cache", "queue", "monitoring"]

# Quality team
quality = ["testing", "security", "performance", "accessibility"]
```

## Best Practices

### Keep Domains Focused

Each domain represents one area of responsibility:

```text
Good: backend, frontend
Bad:  backend-with-all-services, frontend-with-all-build-tools
```

### Avoid Overlapping Domains

If multiple domains need the same rule, put it in root:

```text
Root (shared by all):
├── rules/
│   ├── code-quality.md
│   ├── security.md
│   └── git-workflow.md

Domain-specific:
├── domains/backend/rules/
│   └── database.md
├── domains/frontend/rules/
│   └── component-guidelines.md
```

### Document Domain Purpose

Add comments in config.toml:

```toml
# Domains:
# - backend: Go services, REST APIs, PostgreSQL
# - frontend: React web app, TypeScript
# - mobile: React Native iOS/Android
# - qa: Testing standards
# - devops: Infrastructure, CI/CD, deployment

[profiles]
full = ["backend", "frontend", "mobile", "qa", "devops"]
```

### Use Consistent Names

Use the same domain names across projects for clarity.

### Name Profiles Clearly

Profile names should indicate their purpose:

```text
Good:  full, backend-team, frontend-team
Bad:   p1, p2
```

## Troubleshooting

### Content Not Appearing

Check that your domain is in the profile:

```bash
# List available profiles
ai-rulez validate --debug

# Check config.toml (V4 is TOML: the section is [profiles])
grep -A 5 '^\[profiles\]' .ai-rulez/config.toml
```

### Profile Not Found

```bash
# Validate configuration
ai-rulez validate

# Try generating with debug output
ai-rulez generate --profile backend --debug
```

### Domain Directory Not Recognized

Ensure the directory exists and has content:

```bash
# Check domain directory
ls -la .ai-rulez/domains/backend/

# Domain needs at least one of: rules/, context/, skills/, agents/, commands/
```

### Content Collisions

If both root and domain define the same name:

```bash
# Lists every collapsed duplicate with its kept and dropped source
ai-rulez validate

# The root version wins; remove it if you want the domain version instead
```

## Migration Path

If you're starting with a flat structure:

```bash
# Current structure
.ai-rulez/
├── rules/
│   ├── backend-api-design.md
│   ├── backend-database.md
│   ├── frontend-components.md
│   └── frontend-styling.md
```

To migrate to domains:

1. Create domain structure:

   ```bash
   mkdir -p .ai-rulez/domains/backend/rules
   mkdir -p .ai-rulez/domains/frontend/rules
   ```

2. Move files:

   ```bash
   mv .ai-rulez/rules/backend-* .ai-rulez/domains/backend/rules/
   mv .ai-rulez/rules/frontend-* .ai-rulez/domains/frontend/rules/
   ```

3. Rename files (remove prefix):

   ```bash
   cd .ai-rulez/domains/backend/rules
   mv backend-api-design.md api-design.md
   mv backend-database.md database.md
   ```

4. Update config.toml:

   ```toml
   [profiles]
   full = ["backend", "frontend"]
   backend = ["backend"]
   frontend = ["frontend"]
   ```

5. Test:

   ```bash
   ai-rulez validate
   ai-rulez generate --profile full
   ```

## Next Steps

- **[Quick Start](quick-start.md)**: Getting started with domains
- **[Configuration Reference](configuration.md)**: Advanced config options
- **[Profiles Guide](profiles.md)**: Creating custom presets
