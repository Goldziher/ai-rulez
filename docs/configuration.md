# Configuration Reference

Complete reference for V3 configuration in `.ai-rulez/config.yaml`.

## Basic Structure

The minimal valid V3 configuration:

```yaml
version: "3.0"
name: my-project
```

A typical production configuration:

```yaml
version: "3.0"
name: my-project
description: My project description

presets:
  - claude
  - cursor
  - gemini

default: full

profiles:
  full: [backend, frontend, qa]
  backend: [backend, qa]
  frontend: [frontend, qa]

gitignore: true
```

## Required Fields

### `version`

The V3 schema version. Must be exactly `"3.0"`.

```yaml
version: "3.0"
```

### `name`

The project name. Used in generated files and displayed in headers.

```yaml
name: "My Project"
name: "acme-platform"
name: "backend-api"
```

## Optional Fields

### `description`

Brief description of the project or configuration.

```yaml
description: "SaaS platform with React frontend and Go backend"
```

### `presets`

Specifies which tools to generate configuration for. Can be built-in preset names or custom preset objects.

#### Built-in Presets

```yaml
presets:
  - claude          # → CLAUDE.md
  - cursor          # → .cursorrules
  - gemini          # → GEMINI.md
  - copilot         # → .github/copilot-instructions.md
  - windsurf        # → .windsurf/rules/
  - continue-dev    # → .continue/config.py
  - cline           # → .clinerules/
```

#### Custom Presets

For tools not in the built-in list:

```yaml
presets:
  - name: my-tool
    type: markdown          # or: directory, json
    path: docs/MY_TOOL.md
    template: |
      # {{ .Name }}
      {{ range .Rules }}
      - **{{ .Name }}**: {{ .Content }}
      {{ end }}
```

### `default`

The default profile name used when `ai-rulez generate` is run without `--profile`.

```yaml
default: full
```

If not specified, all domains are included.

### `profiles`

Named profiles that specify which domains to include in generation.

```yaml
profiles:
  full:
    - backend
    - frontend
    - qa
  backend:
    - backend
    - qa
  frontend:
    - frontend
    - qa
  qa:
    - qa
```

Each profile is a list of domain names. When generating with a profile:
1. All root content (`.ai-rulez/rules/`, `.ai-rulez/context/`, `.ai-rulez/skills/`) is included
2. Content from specified domains (`.ai-rulez/domains/{name}/`) is included

### `gitignore`

Controls whether `ai-rulez` automatically updates `.gitignore` with generated files.

```yaml
gitignore: true    # Default: update .gitignore automatically
gitignore: false   # Manual .gitignore management
```

When `true`, generated files are added to `.gitignore` to prevent accidental commits.

## Directory Structure

### Root Content (Always Included)

Content in these directories is always included in every generation:

```
.ai-rulez/
├── rules/           # Mandatory rules and constraints
├── context/         # Reference documentation
└── skills/          # AI skills/prompts
```

**Rules directory**: `rules/`
- Files: `*.md` markdown files
- Purpose: Mandatory constraints, standards, do's and don'ts
- Included: in all generated outputs

**Context directory**: `context/`
- Files: `*.md` markdown files
- Purpose: Reference documentation, architecture, guidelines
- Included: in all generated outputs

**Skills directory**: `skills/`
- Structure: `skills/{skill-name}/SKILL.md`
- Purpose: Specialized AI prompts and expert prompts
- Included: in all generated outputs

### Domain Content (Profile-Specific)

Content in domain directories is included only when that domain is in the active profile:

```
.ai-rulez/domains/
├── backend/
│   ├── rules/
│   ├── context/
│   └── skills/
├── frontend/
│   ├── rules/
│   ├── context/
│   └── skills/
└── qa/
    ├── rules/
    └── context/
```

Domain directories mirror the root structure:
- `domains/{name}/rules/` - Domain-specific rules
- `domains/{name}/context/` - Domain-specific documentation
- `domains/{name}/skills/` - Domain-specific AI skills

## File Formats

### Markdown Files

All `.md` files are treated as content. Optional YAML frontmatter is supported:

```markdown
---
priority: high
targets:
  - "*.py"
  - "backend/*"
custom_field: value
---

# Rule or Context Title

Your content here. Can include any markdown formatting.
```

### Frontmatter Fields

**`priority`** (optional, string)
- Values: `critical`, `high`, `medium`, `low`, `minimal`
- Default: `medium`
- Controls sort order in generated files (higher priority first)

```yaml
---
priority: critical
---
```

**`targets`** (optional, array of strings)
- File glob patterns specifying which generated outputs include this content
- If empty, included in all outputs

```yaml
---
targets:
  - "CLAUDE.md"
  - ".cursor/rules/*"
---
```

**Custom fields** (optional)
- Any other YAML fields are preserved and available in custom templates

```yaml
---
priority: high
author: engineering-team
review_date: 2025-01-01
tags: [security, performance]
---
```

### SKILL.md Format

Skills should follow this structure:

```markdown
---
priority: high
description: "Code reviewer expert for quality assurance"
targets: ["CLAUDE.md"]
---

# Code Reviewer Expert

You are an expert code reviewer with deep knowledge of:
- Code quality and maintainability
- Testing best practices
- Performance optimization

## Your Responsibilities

1. Review pull requests for correctness
2. Suggest improvements and refactoring
3. Verify test coverage
```

## Content Merge Strategy

When generating with a profile, content is merged in this order:

1. **Root rules** (`.ai-rulez/rules/`)
2. **Root context** (`.ai-rulez/context/`)
3. **Root skills** (`.ai-rulez/skills/`)
4. **Domain rules** (for each domain in profile)
5. **Domain context** (for each domain in profile)
6. **Domain skills** (for each domain in profile)

Within each category, files are sorted by:
1. **Priority** (critical → high → medium → low → minimal)
2. **Filename** (alphabetical)

### Name Collision Handling

If a filename appears in both root and domain:

```
.ai-rulez/rules/testing.md
.ai-rulez/domains/backend/rules/testing.md
```

The domain version takes precedence (backend gets the domain-specific version).

A warning is logged if collisions are detected:
```
⚠️  Content collision: rules/testing.md exists in both root and backend domain
    → Using backend domain version
```

## Configuration Examples

### Small Project (Single Team)

```yaml
version: "3.0"
name: "My Startup"
description: "Early-stage SaaS with React + Go"

presets:
  - claude
  - cursor

gitignore: true
```

Directory structure:
```
.ai-rulez/
├── config.yaml
├── rules/
│   ├── code-style.md
│   └── testing.md
├── context/
│   └── architecture.md
└── skills/
    └── code-reviewer/
        └── SKILL.md
```

### Medium Project (Multiple Teams)

```yaml
version: "3.0"
name: "Enterprise Platform"
description: "Multi-team SaaS platform"

presets:
  - claude
  - cursor
  - gemini

default: full

profiles:
  full: [backend, frontend, qa, devops]
  backend: [backend, qa]
  frontend: [frontend, qa]
  qa: [qa]
  devops: [devops]

gitignore: true
```

Directory structure:
```
.ai-rulez/
├── config.yaml
├── rules/
│   ├── general-standards.md
│   └── security.md
└── domains/
    ├── backend/
    │   ├── rules/
    │   │   ├── api-design.md
    │   │   └── database.md
    │   └── context/
    │       └── backend-architecture.md
    ├── frontend/
    │   ├── rules/
    │   │   ├── component-guidelines.md
    │   │   └── performance.md
    │   └── context/
    │       └── design-system.md
    ├── qa/
    │   └── rules/
    │       └── testing-strategy.md
    └── devops/
        ├── rules/
        │   └── deployment.md
        └── context/
            └── infrastructure.md
```

### Complex Project (Multiple Presets)

```yaml
version: "3.0"
name: "Advanced ML Platform"
description: "Research platform with team separation"

presets:
  - claude
  - cursor
  - gemini
  - windsurf
  - name: internal-guide
    type: markdown
    path: docs/AI_DEVELOPMENT_GUIDE.md

default: full

profiles:
  full: [research, ml-ops, infrastructure, frontend]
  research: [research]
  ml-ops: [ml-ops, infrastructure]
  frontend: [frontend]

gitignore: true
```

## Profile Design Patterns

### Single Team (No Domains)

For projects with a single team, skip domains entirely:

```yaml
version: "3.0"
name: simple-project
presets:
  - claude
  - cursor
```

### Multi-Team Monorepo

For monorepos with multiple independent teams:

```yaml
version: "3.0"
name: platform
presets:
  - claude
  - cursor

default: full

profiles:
  full: [backend, frontend, mobile]
  backend: [backend]
  frontend: [frontend]
  mobile: [mobile]
```

### Environment-Based Profiles

For different behavior in dev, staging, production:

```yaml
version: "3.0"
name: saas-app
presets:
  - claude

default: production

profiles:
  development: [dev-guidelines]
  staging: [staging-guidelines]
  production: [production-guidelines, security-hardened]
```

## Validation

V3 configurations are validated against the JSON schema:

```
schema/ai-rules-v3.schema.json
```

To validate your configuration:

```bash
ai-rulez validate
```

This checks:
- `version` is exactly `"3.0"`
- `name` is present and non-empty
- All preset names are valid
- File paths are valid

## Tips and Best Practices

### Use Meaningful Domain Names

Good domain names:
- `backend`, `frontend`, `mobile` (service boundaries)
- `api`, `database`, `queue` (technical domains)
- `auth`, `payments`, `search` (feature areas)

Avoid:
- `team1`, `team2` (not descriptive)
- Single letters or acronyms without context

### Organize by Responsibility

Put content in the domain that owns it:

```
Good:
- domains/backend/rules/database-standards.md
- domains/frontend/rules/accessibility-guidelines.md

Bad:
- domains/shared/rules/database-standards.md  (ambiguous ownership)
- domains/qa/rules/everything.md               (too broad)
```

### Keep Domains Focused

Domains should represent one area of responsibility:

```
Preferred:
profiles:
  full: [api, frontend, infrastructure]

Less ideal:
profiles:
  full: [api-with-database, frontend-with-build-tools, infrastructure-and-monitoring]
```

### Document Domain Purposes

Add a comment in `config.yaml`:

```yaml
# Domains:
# - backend: Go microservices, REST APIs
# - frontend: React web application
# - mobile: React Native mobile apps
# - qa: Testing standards for all platforms
# - devops: Infrastructure and deployment

profiles:
  full: [backend, frontend, mobile, qa, devops]
```

## Troubleshooting

### "Profile not found"

```bash
# Check which profiles are defined
ai-rulez validate

# Try generating with an explicit profile
ai-rulez generate --profile backend
```

### "No presets specified"

At least one preset is recommended. Add to config:

```yaml
presets:
  - claude
```

### "Domain not included"

Check your profile configuration:

```yaml
# If this profile doesn't include "backend", backend content won't appear
profiles:
  myprofile:
    - frontend      # backend is missing!
    - qa
```

### Content not appearing in output

1. Verify domain is in profile:
   ```bash
   ai-rulez validate  # Check profile definitions
   ```

2. Check file location:
   - Root: `.ai-rulez/rules/`, `.ai-rulez/context/`, `.ai-rulez/skills/`
   - Domain: `.ai-rulez/domains/{name}/rules/`, etc.

3. Check for frontmatter errors:
   - Invalid YAML in `---` blocks will skip the file
   - Remove frontmatter to test

4. Regenerate explicitly:
   ```bash
   ai-rulez generate --profile your-profile
   ```

## Next Steps

- **[Getting Started](quick-start.md)**: Quick start and common patterns
- **[CLI Reference](cli.md)**: All commands and flags
- **[Domains & Profiles](domains.md)**: Team organization patterns
