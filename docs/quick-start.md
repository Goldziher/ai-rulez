# Quick Start

Get AI-Rulez running in 5 minutes.

## Step 1: Initialize Your Project

Create a new V3 configuration:

```bash
ai-rulez init "my-project" --v3
```

This creates a `.ai-rulez/` directory with:
```
.ai-rulez/
├── config.yaml
├── rules/
│   └── example-rule.md
├── context/
│   └── example-context.md
├── skills/
│   └── code-reviewer/
│       └── SKILL.md
├── agents/
└── mcp.yaml
```

## Step 2: Configure Your Presets

Edit `.ai-rulez/config.yaml` to specify which tools to generate for:

```yaml
version: "3.0"
name: "my-project"
description: "My awesome project"

# Tools to generate configuration for
presets:
  - claude           # Generates CLAUDE.md
  - cursor           # Generates .cursor/rules/
  - gemini           # Generates GEMINI.md

# Default profile when none specified
default: full

# Named profiles for different team needs
profiles:
  full:
    - []             # Empty = root content only
```

## Step 3: Add Your Rules

Create rule files in `.ai-rulez/rules/`:

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

**`.ai-rulez/rules/git-workflow.md`:**
```markdown
---
priority: medium
---

# Git Workflow

1. Feature branches from main
2. Squash commits before merge
3. Require code review before merge
```

## Step 4: Add Context Documentation

Create context files in `.ai-rulez/context/`:

**`.ai-rulez/context/architecture.md`:**
```markdown
# Architecture

## System Design
- 3 microservices behind an API Gateway
- PostgreSQL for persistence
- Kubernetes for orchestration

## Stack
- Backend: Go
- Frontend: React
- Infrastructure: Kubernetes
```

## Step 5: Define Skills (Optional)

Create specialized AI prompts in `.ai-rulez/skills/`:

**`.ai-rulez/skills/code-reviewer/SKILL.md`:**
```markdown
---
priority: high
description: "Code reviewer for quality assurance"
---

# Code Reviewer

Review code for:
- Quality and maintainability
- Test coverage
- Performance issues

Responsibilities:
1. Review pull requests for correctness
2. Suggest improvements
3. Verify test coverage
```

## Step 6: Generate Outputs

Generate configuration files for all your tools:

```bash
ai-rulez generate
```

This creates:
- `CLAUDE.md` (from claude preset)
- `.cursor/rules/` (from cursor preset)
- `GEMINI.md` (from gemini preset)

## Step 7: Verify and Commit

Check that files were generated:

```bash
ls -la CLAUDE.md .cursor/rules/ GEMINI.md
```

Commit everything:

```bash
git add .ai-rulez/ CLAUDE.md .cursor/rules/ GEMINI.md
git commit -m "docs: initialize AI assistant configuration"
```

## Multi-Team Setup

For projects with multiple teams, add domains:

**1. Create domain structure:**
```bash
mkdir -p .ai-rulez/domains/backend/rules
mkdir -p .ai-rulez/domains/frontend/rules
```

**2. Add domain-specific rules:**

**`.ai-rulez/domains/backend/rules/database.md`:**
```markdown
---
priority: critical
---

# Database Standards

- Use prepared statements
- Add migrations for schema changes
- Index foreign keys
```

**3. Update `config.yaml`:**
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

**4. Generate for specific teams:**
```bash
# Backend team gets root + backend content
ai-rulez generate --profile backend

# Frontend team gets root + frontend content
ai-rulez generate --profile frontend

# CI/QA gets everything
ai-rulez generate --profile full
```

## Common Tasks

### Update Rules

Edit any file in `.ai-rulez/rules/` and regenerate:

```bash
# Edit a rule
vim .ai-rulez/rules/code-quality.md

# Regenerate outputs
ai-rulez generate
```

### Add a New Domain

```bash
mkdir -p .ai-rulez/domains/newdomain/rules
mkdir -p .ai-rulez/domains/newdomain/context

# Add rules and context files...

# Update config.yaml to reference the domain
```

### Change Tool Configuration

Edit presets in `config.yaml`:

```yaml
presets:
  - claude
  - cursor
  - windsurf        # Add Windsurf
  - copilot         # Add Copilot
```

### Create Custom Output

For tools not in the built-in list:

```yaml
presets:
  - claude
  - name: my-tool
    type: markdown
    path: docs/MY_TOOL.md
```

## Troubleshooting

### Generated files aren't updating

Make sure you ran `ai-rulez generate`:
```bash
ai-rulez generate
```

### Content not appearing in output

Check that your file is in the correct location:
- Root content: `.ai-rulez/rules/`, `.ai-rulez/context/`
- Domain content: `.ai-rulez/domains/{name}/rules/`, etc.

### Validation fails

Check your configuration:
```bash
ai-rulez validate
```

This will show errors in your setup.

## Next Steps

- **[Configuration Reference](configuration.md)**: Learn all config options
- **[Domains & Profiles](domains.md)**: Organize by team
- **[Custom Presets](profiles.md)**: Generate for custom tools
- **[Includes System](includes.md)**: Share configs across projects
