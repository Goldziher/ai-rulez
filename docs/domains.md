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

```yaml
profiles:
  full: [backend, frontend, qa]        # All teams
  backend: [backend, qa]               # Backend team only
  frontend: [frontend, qa]             # Frontend team only
  qa: [qa]                             # QA team only
```

Generation always includes:
- All root content (rules, context, skills)
- Content from selected domains only

## Directory Structure

```
.ai-rulez/
├── config.yaml              # Main config with presets and profiles
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

```yaml
domains:
  - api: REST API service
  - database: Database layer
  - cache: Caching layer
  - queue: Message queue
  - frontend: Web UI
```

**Profiles:**
```yaml
profiles:
  full: [api, database, cache, queue, frontend]
  backend: [api, database, cache, queue]
  frontend: [frontend]
  infrastructure: [database, cache, queue]
```

### Team-Based Domains

For organizations with dedicated teams:

```yaml
domains:
  - backend: Go microservices
  - frontend: React web app
  - mobile: React Native mobile
  - qa: Testing and quality assurance
  - devops: Infrastructure and deployment
```

**Profiles:**
```yaml
profiles:
  full: [backend, frontend, mobile, qa, devops]
  backend-team: [backend, qa]
  frontend-team: [frontend, qa]
  mobile-team: [mobile, qa]
  qa-team: [qa]
  devops-team: [devops]
  ci-all: [backend, frontend, mobile, qa, devops]
```

### Feature-Based Domains

For projects organized by feature:

```yaml
domains:
  - auth: Authentication and authorization
  - payments: Payment processing
  - notifications: Email, SMS, push notifications
  - search: Search and indexing
  - analytics: Data collection and analysis
```

**Profiles:**
```yaml
profiles:
  full: [auth, payments, notifications, search, analytics]
  backend: [auth, payments, notifications, search, analytics]
  frontend: [notifications, search]
```

### Environment-Based Domains

For different rules per environment:

```yaml
domains:
  - dev: Development guidelines
  - staging: Staging constraints
  - prod: Production rules
  - security-hardened: Extra security measures
```

**Profiles:**
```yaml
profiles:
  development: [dev]
  staging: [staging, security-hardened]
  production: [prod, security-hardened]
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

### Step 3: Update config.yaml

```yaml
version: "3.0"
name: "my-platform"

presets:
  - claude
  - cursor

default: full

profiles:
  full:
    - backend
    - frontend
  backend:
    - backend
  frontend:
    - frontend
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

```yaml
profiles:
  full-stack:
    - backend
    - frontend
```

```bash
ai-rulez generate --profile full-stack
# Gets: root + backend + frontend content
```

## Domain Content Priority

When a file exists in both root and domain:

```
.ai-rulez/rules/testing.md              (root)
.ai-rulez/domains/backend/rules/testing.md   (domain)
```

The **domain version takes precedence** for that domain.

```
backend profile gets:
  - everything from .ai-rulez/rules/* EXCEPT testing.md
  - testing.md from .ai-rulez/domains/backend/rules/

frontend profile gets:
  - everything from .ai-rulez/rules/* (including testing.md)
  - frontend-specific rules from domains/frontend/
```

## Advanced Profile Combinations

### Multi-Level Profiles

Profiles can include multiple domains with shared subsets:

```yaml
profiles:
  # Full stack for dev team
  full-dev: [backend, frontend, devops, qa]

  # Minimal for contractors
  frontend-only: [frontend]

  # Security-focused for compliance
  security-audit: [security, backend, database]

  # Performance optimization
  perf-team: [backend, database, cache]
```

### Environment-Specific Profiles

```yaml
profiles:
  # Development: loose constraints
  dev:
    - dev-guidelines
    - logging-verbose

  # Staging: stricter
  staging:
    - staging-checks
    - logging-standard
    - security-checks

  # Production: strictest
  production:
    - production-critical
    - logging-minimal
    - security-hardened
    - compliance
```

### Feature-Based Selection

```yaml
profiles:
  # New features team
  features:
    - feature-auth
    - feature-payments
    - feature-notifications

  # Infrastructure team
  infrastructure:
    - database
    - cache
    - queue
    - monitoring

  # Quality team
  quality:
    - testing
    - security
    - performance
    - accessibility
```

## Best Practices

### Keep Domains Focused

Each domain represents one area of responsibility:

```yaml
Good:
domains:
  - backend
  - frontend

Bad:
domains:
  - backend-with-all-services
  - frontend-with-all-build-tools
```

### Avoid Overlapping Domains

If multiple domains need the same rule, put it in root:

```
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

Add comments in config.yaml:

```yaml
# Domains:
# - backend: Go services, REST APIs, PostgreSQL
# - frontend: React web app, TypeScript
# - mobile: React Native iOS/Android
# - qa: Testing standards
# - devops: Infrastructure, CI/CD, deployment

profiles:
  full: [backend, frontend, mobile, qa, devops]
```

### Use Consistent Names

Use the same domain names across projects for clarity.

### Name Profiles Clearly

Profile names should indicate their purpose:

```yaml
Good:
profiles:
  full: [all domains]
  backend-team: [backend, shared-qa]
  frontend-team: [frontend, shared-qa]

Bad:
profiles:
  p1: [backend, frontend]
  p2: [backend]
```

## Troubleshooting

### Content Not Appearing

Check that your domain is in the profile:

```bash
# List available profiles
ai-rulez validate --verbose

# Check config.yaml
cat .ai-rulez/config.yaml | grep -A 5 "profiles:"
```

### Profile Not Found

```bash
# Validate configuration
ai-rulez validate

# Try generating with verbose output
ai-rulez generate --profile backend --verbose
```

### Domain Directory Not Recognized

Ensure the directory exists and has content:

```bash
# Check domain directory
ls -la .ai-rulez/domains/backend/

# Domain needs at least one of: rules/, context/, skills/
```

### Content Collisions

If both root and domain have the same file:

```bash
# Check which version is used
ai-rulez validate --verbose

# Domain version takes precedence
# Remove from root if you want domain-specific only
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

4. Update config.yaml:
   ```yaml
   profiles:
     full: [backend, frontend]
     backend: [backend]
     frontend: [frontend]
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
