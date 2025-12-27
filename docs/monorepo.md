# Best Practices for Large Projects

For large projects with multiple teams, organize your `.ai-rulez/` configuration using domains and profiles to provide relevant context while avoiding overwhelming AI assistants with irrelevant information.

This guide outlines best practices for managing AI context at scale in V3.

---

## The Core Strategy: Domain-Based Organization

The most effective strategy for large projects is to organize related rules, context, and skills into domains. Each domain represents a team, service, or feature area.

!!! success "The Goal: High-Relevance, Low-Token Context"

    By organizing into domains and using profiles, you provide your AI assistant with only the context it needs for the task at hand. A developer working on the frontend gets frontend-specific rules and context, while the backend developer gets API patterns and database standards. This results in faster, more accurate, and more relevant AI responses.

!!! info "Domains and Profiles"
    - **Domains** organize content by team, service, or feature (`backend`, `frontend`, `mobile`)
    - **Profiles** select which domains are included in generation (`full`, `backend-team`, etc.)

A typical layout might look like this:

```
my-project/
└── .ai-rulez/
    ├── config.yaml              # ⬅️ Main config: presets, profiles, domains
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

**`.ai-rulez/config.yaml`:**
```yaml
version: "3.0"
name: "My Full-Stack Project"
description: "Microservices project with React frontend and Go backend"

presets:
  - claude
  - cursor
  - gemini

default: full

profiles:
  full: [backend, frontend, qa]
  backend: [backend, qa]
  frontend: [frontend, qa]
  qa: [qa]

gitignore: true
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

```yaml
profiles:
  # Full platform: all content
  full: [backend, frontend, qa, devops]

  # Team-specific
  backend-team: [backend, qa]
  frontend-team: [frontend, qa]
  qa-team: [qa]
  devops-team: [devops]

  # Full-stack developers
  full-stack: [backend, frontend, qa]

  # CI/QA environment
  ci-cd: [backend, frontend, qa, devops]
```

## Best Practice: For Monorepos

If using multiple `.ai-rulez/` directories in a monorepo, use the `--recursive` flag:

```bash
# Process all .ai-rulez/ directories recursively
ai-rulez generate --recursive
```

**Root configuration** (`/.ai-rulez/config.yaml`):
```yaml
version: "3.0"
name: "Monorepo Platform"

presets:
  - claude
  - cursor

default: full

profiles:
  full: [shared]
```

**Service-specific** (`/backend/.ai-rulez/config.yaml`):
```yaml
version: "3.0"
name: "Backend Service"

presets:
  - claude

default: backend

profiles:
  backend: [api, database]
```

By combining domain organization with thoughtful profile design, you can create scalable, maintainable configurations that grow with your project.
