# Configuration Examples

V3 uses a file-based directory structure (`.ai-rulez/`) instead of a single YAML file. These examples show how to organize your configuration across multiple markdown and YAML files.

---

## 1. Minimal Configuration (Single Team)

The simplest V3 setup for a single team with basic rules.

**`.ai-rulez/config.yaml`:**
```yaml
version: "3.0"
name: "My Project"

presets:
  - claude
  - cursor

gitignore: true
```

**`.ai-rulez/rules/code-quality.md`:**
```markdown
---
priority: high
---

# Code Quality

- Use meaningful variable names
- Comment complex logic
- All tests must pass before merge
```

**`.ai-rulez/context/architecture.md`:**
```markdown
# Architecture

This is a monolithic application with:
- PostgreSQL database
- REST API backend
- React frontend
```

### Generated Output
```bash
ai-rulez generate
# Creates:
# - CLAUDE.md
# - .cursor/rules/
```

---

## 2. Multi-Team Configuration with Domains

For projects with multiple teams, use domains to organize team-specific content.

**`.ai-rulez/config.yaml`:**
```yaml
version: "3.0"
name: "Platform"

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

**`.ai-rulez/rules/security.md`:**
```markdown
---
priority: critical
---

# Security

- All secrets must use environment variables
- Validate all user input
- Use HTTPS for all external API calls
```

**`.ai-rulez/domains/backend/rules/database.md`:**
```markdown
---
priority: critical
---

# Database Standards

- Use prepared statements to prevent SQL injection
- Always add database migrations
- Index foreign keys for performance
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
```

### Generated Output
```bash
# Generate for backend team
ai-rulez generate --profile backend
# Includes: root rules + backend-specific rules

# Generate for frontend team
ai-rulez generate --profile frontend
# Includes: root rules + frontend-specific rules

# Generate for QA
ai-rulez generate --profile qa
# Includes: root rules + QA-specific rules
```

---

## 3. Using Skills for Specialized Roles

Create AI skill definitions for specialized tasks.

**`.ai-rulez/skills/code-reviewer/SKILL.md`:**
```markdown
---
priority: high
description: "Code reviewer for quality assurance"
---

# Code Reviewer

You are an expert code reviewer with deep knowledge of:
- Code quality and maintainability
- Testing best practices
- Performance optimization

## Your Responsibilities

1. Review pull requests for correctness
2. Suggest improvements and refactoring
3. Verify test coverage
```

**`.ai-rulez/skills/architecture-expert/SKILL.md`:**
```markdown
---
priority: high
description: "System architecture specialist"
---

# Architecture Expert

You are a system architect specializing in:
- Microservices design
- Scalability patterns
- System reliability

## Your Responsibilities

1. Review architectural decisions
2. Suggest performance improvements
3. Identify technical debt
```

### Usage in Generated Files

The generated CLAUDE.md will include instructions like:

```
Use the @code-reviewer skill for pull request reviews.
Use the @architecture-expert skill for design questions.
```

---

## 4. Complex Project with Multiple Presets

For projects that need different output formats for different tools.

**`.ai-rulez/config.yaml`:**
```yaml
version: "3.0"
name: "ML Research Platform"
description: "Machine learning platform with team separation"

presets:
  - claude         # → CLAUDE.md
  - cursor         # → .cursor/rules/
  - gemini         # → GEMINI.md
  - windsurf       # → .windsurf/rules/
  - name: internal-guide
    type: markdown
    path: docs/AI_DEVELOPMENT_GUIDE.md

default: full

profiles:
  full: [research, infrastructure]
  research: [research]
  infrastructure: [infrastructure]

gitignore: true
```

**`.ai-rulez/domains/research/rules/ml-standards.md`:**
```markdown
---
priority: critical
targets: ["CLAUDE.md", "GEMINI.md"]
---

# ML Development Standards

- Use type hints for all functions
- Document mathematical assumptions
- Include reproducibility seeds
```

**`.ai-rulez/domains/infrastructure/rules/deployment.md`:**
```markdown
---
priority: high
targets: ["CLAUDE.md", ".cursor/rules/"]
---

# Deployment Standards

- All changes require review
- Run tests before deployment
- Keep infrastructure as code
```

---

## 5. Project with Frontmatter and Custom Fields

Markdown files can include YAML frontmatter with custom fields.

**`.ai-rulez/rules/testing.md`:**
```markdown
---
priority: critical
author: qa-team
tags: [testing, quality, ci-cd]
review_date: 2025-01-15
targets:
  - "CLAUDE.md"
  - ".cursor/rules/"
---

# Testing Standards

## Unit Tests

All code changes must include corresponding unit tests.

- Aim for 80%+ code coverage
- Use table-driven tests for Go
- Test both happy path and error cases

## Integration Tests

Test service interactions:
- Database operations
- API endpoints
- External service calls
```

---

## 6. Monorepo with Shared Rules

For larger projects, reuse configurations across subdirectories.

**`/.ai-rulez/config.yaml`** (Root config):
```yaml
version: "3.0"
name: "Platform"

presets:
  - claude
  - cursor

default: full

profiles:
  full: [shared]
```

**`/backend/.ai-rulez/config.yaml`** (Backend-specific):
```yaml
version: "3.0"
name: "Backend Service"

presets:
  - claude
  - cursor

default: backend

profiles:
  backend: [api, database]
```

**`/backend/.ai-rulez/domains/api/rules/endpoints.md`:**
```markdown
---
priority: high
---

# API Endpoint Guidelines

- Use consistent path structure
- Version APIs from the start
- Return consistent error responses
```

### Generation
```bash
# From root, processes all .ai-rulez/ directories recursively
ai-rulez generate --recursive
```

---

## 7. Environment-Specific Profiles

Use profiles for different deployment environments.

**`.ai-rulez/config.yaml`:**
```yaml
version: "3.0"
name: "Web Application"

presets:
  - claude

profiles:
  development:
    - dev-guidelines
  staging:
    - staging-checks
    - security-checks
  production:
    - production-critical
    - security-hardened
    - compliance
```

**`.ai-rulez/domains/dev-guidelines/rules/debugging.md`:**
```markdown
---
priority: medium
---

# Development Guidelines

- Enable verbose logging in dev
- Use debug endpoints for testing
- Performance is less critical than clarity
```

**`.ai-rulez/domains/production-critical/rules/reliability.md`:**
```markdown
---
priority: critical
---

# Production Standards

- All deployments require approval
- Monitor error rates in production
- Implement circuit breakers for external services
```

### Usage
```bash
# Generate for development
ai-rulez generate --profile development

# Generate for production
ai-rulez generate --profile production
```
