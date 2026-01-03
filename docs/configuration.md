# Configuration Reference

V3 configuration reference for `.ai-rulez/config.yaml`.

## File-Based Configuration

V3 uses a file-based approach where you edit files directly with your editor or use CRUD commands:

- **Configuration**: Edit `.ai-rulez/config.yaml` with any text editor
- **Rules**: Add/edit `.ai-rulez/rules/*.md` files or use `ai-rulez add rule`
- **Context**: Add/edit `.ai-rulez/context/*.md` files or use `ai-rulez add context`
- **Skills**: Add/edit `.ai-rulez/skills/{name}/SKILL.md` files or use `ai-rulez add skill`
- **Agents**: Add/edit `.ai-rulez/agents/*.md` files or use `ai-rulez add agent`
- **Domains**: Add/edit `.ai-rulez/domains/{name}/{rules,context,skills,agents}/*.md` files or use `ai-rulez domain add`

You can either directly edit files with your editor or use CRUD commands for programmatic modification. After changes, run `ai-rulez generate` to create tool-specific outputs.

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
1. All root content (`.ai-rulez/rules/`, `.ai-rulez/context/`, `.ai-rulez/skills/`, `.ai-rulez/agents/`) is included
2. Content from specified domains (`.ai-rulez/domains/{name}/`) is included

### `gitignore`

Controls whether `ai-rulez` automatically updates `.gitignore` with generated files.

```yaml
gitignore: true    # Default: update .gitignore automatically
gitignore: false   # Manual .gitignore management
```

When `true`, generated files are added to `.gitignore` to prevent accidental commits.

### `header`

Configures the style of headers in generated files. Headers provide context about ai-rulez, explain the folder structure, and instruct AI agents on proper usage.

```yaml
header:
  style: detailed    # Default: comprehensive header with full documentation
  style: compact     # Shorter header with key information
  style: minimal     # Bare minimum header
```

#### Header Styles

**`detailed`** (default)
- Comprehensive explanation of ai-rulez
- Complete folder organization documentation
- Full AI agent instructions with MCP server promotion
- Best for: projects where AI agents need thorough context
- Size: ~50 lines

**`compact`**
- Condensed version with essential information
- Uses symbols (✗/✓) for clarity
- Brief structure overview
- Best for: projects where file size matters
- Size: ~20 lines

**`minimal`**
- Only critical information
- Brief "DO NOT EDIT" warning
- MCP server reference
- Best for: projects with strict file size requirements
- Size: ~10 lines

#### Header Example (detailed)

```html
<!--
🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT DIRECTLY
Project: My Project
Generated: 2026-01-03 09:27:19
Source: .ai-rulez/config.yaml
Target: CLAUDE.md
Content: rules=5, sections=0, agents=2

WHAT IS AI-RULEZ
AI-Rulez is a directory-based AI governance tool. All configuration lives in
the .ai-rulez/ directory. This file is auto-generated from source files.

.AI-RULEZ FOLDER ORGANIZATION
Root content (always included):
  .ai-rulez/config.yaml    Main configuration (presets, profiles)
  .ai-rulez/rules/         Mandatory rules for AI assistants
  .ai-rulez/context/       Reference documentation
  .ai-rulez/skills/        Specialized AI prompts
  .ai-rulez/agents/        Agent definitions

Domain content (profile-specific):
  .ai-rulez/domains/{name}/rules/    Domain-specific rules
  .ai-rulez/domains/{name}/context/  Domain-specific documentation
  .ai-rulez/domains/{name}/skills/   Domain-specific AI prompts

Profiles in config.yaml control which domains are included.

INSTRUCTIONS FOR AI AGENTS
1. NEVER edit this file (CLAUDE.md) - it is auto-generated

2. ALWAYS edit files in .ai-rulez/ instead:
   - Add/modify rules: .ai-rulez/rules/*.md
   - Add/modify context: .ai-rulez/context/*.md
   - Update config: .ai-rulez/config.yaml
   - Domain-specific: .ai-rulez/domains/{name}/rules/*.md

3. PREFER using the MCP Server (if available):
   Command: npx -y ai-rulez@latest mcp
   Provides safe CRUD tools for reading and modifying .ai-rulez/ content

4. After making changes: ai-rulez generate

5. Complete workflow:
   a. Edit source files in .ai-rulez/
   b. Run: ai-rulez generate
   c. Commit both .ai-rulez/ and generated files

Documentation: https://github.com/Goldziher/ai-rulez
-->
```

#### Header Example (compact)

```html
<!--
🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT
Project: My Project | Generated: 2026-01-03 09:28:08
Source: .ai-rulez/config.yaml | Target: CLAUDE.md
Content: rules=5, sections=0, agents=2

WHAT IS AI-RULEZ: Directory-based AI governance. Config in .ai-rulez/

STRUCTURE:
  .ai-rulez/config.yaml, rules/, context/, skills/, agents/ (root)
  .ai-rulez/domains/{name}/ (profile-specific)

AI AGENT INSTRUCTIONS:
✗ NEVER edit CLAUDE.md (auto-generated)
✓ EDIT .ai-rulez/rules/*.md, .ai-rulez/context/*.md, .ai-rulez/config.yaml
✓ USE MCP server: npx -y ai-rulez@latest mcp (provides CRUD tools)
✓ REGENERATE: ai-rulez generate
✓ COMMIT: both .ai-rulez/ and generated files

Docs: https://github.com/Goldziher/ai-rulez
-->
```

#### Header Example (minimal)

```html
<!--
🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT
Project: My Project
Generated: 2026-01-03 09:28:27
Source: .ai-rulez/config.yaml

NEVER edit this file - modify .ai-rulez/ content instead
Use MCP server: npx -y ai-rulez@latest mcp
Regenerate: ai-rulez generate

Docs: https://github.com/Goldziher/ai-rulez
-->
```

## Directory Structure

### Root Content (Always Included)

Content in these directories is always included in every generation:

```
.ai-rulez/
├── rules/           # Mandatory rules and constraints
├── context/         # Reference documentation
├── skills/          # AI skills/prompts
└── agents/          # Agent prompts
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

**Agents directory**: `agents/`
- Files: `*.md` markdown files
- Purpose: Agent prompt files for supported tools
- Included: in all generated outputs

### Domain Content (Profile-Specific)

Content in domain directories is included only when that domain is in the active profile:

```
.ai-rulez/domains/
├── backend/
│   ├── rules/
│   ├── context/
│   ├── skills/
│   └── agents/
├── frontend/
│   ├── rules/
│   ├── context/
│   ├── skills/
│   └── agents/
└── qa/
    ├── rules/
    ├── context/
    └── agents/
```

Domain directories mirror the root structure:
- `domains/{name}/rules/` - Domain-specific rules
- `domains/{name}/context/` - Domain-specific documentation
- `domains/{name}/skills/` - Domain-specific AI skills
- `domains/{name}/agents/` - Domain-specific agent prompts

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

## Programmatic Modification with CRUD Operations

V3 provides CRUD (Create, Read, Update, Delete) commands to programmatically modify your configuration. This is useful for:

- Automation and scripting
- Integration with CI/CD pipelines
- Programmatic domain and rule management
- Integration with AI assistants via MCP tools

### Manual File Editing vs CRUD Commands

**Manual file editing:**
- Direct control over content
- Use any text editor
- Better for complex content
- Version control friendly

**CRUD commands:**
- Automated directory structure creation
- Frontmatter generation
- Validation built-in
- Easier for scripting and automation
- Better for programmatic access

### Domain Structure with CRUD

When you create a domain with `ai-rulez domain add`, the following structure is automatically created:

```
.ai-rulez/domains/my-domain/
├── rules/           # Domain-specific rules
├── context/         # Domain-specific documentation
└── skills/          # Domain-specific AI skills
```

This mirrors the root structure and allows you to organize content by ownership.

### CRUD Command Categories

**Domain Management:**
```bash
ai-rulez domain add <name>           # Create a domain
ai-rulez domain remove <name>        # Delete a domain
ai-rulez domain list                 # List all domains
```

**Content Management:**
```bash
# Add content to root or domain
ai-rulez add rule <name>             # Create a rule
ai-rulez add context <name>          # Create context
ai-rulez add skill <name>            # Create a skill

# Remove content
ai-rulez remove rule <name>          # Delete a rule
ai-rulez remove context <name>       # Delete context
ai-rulez remove skill <name>         # Delete a skill

# List content
ai-rulez list rules                  # List all rules
ai-rulez list context                # List all context
ai-rulez list skills                 # List all skills
```

**Include Management:**
```bash
ai-rulez include add <name> <source> # Add an include source
ai-rulez include remove <name>       # Remove an include
ai-rulez include list                # List all includes
```

**Profile Management:**
```bash
ai-rulez profile add <name> <domains>      # Create a profile
ai-rulez profile remove <name>             # Delete a profile
ai-rulez profile set-default <name>        # Set default profile
ai-rulez profile list                      # List all profiles
```

### Frontmatter Generation

When adding content via CRUD commands, frontmatter is automatically generated:

```markdown
---
priority: medium
targets: []
---

Your content here...
```

You can override defaults:

```bash
ai-rulez add rule my-rule --priority high --targets claude,cursor
```

### Best Practices for Configuration Management

1. **Use CRUD for automation**: Scripts, CI/CD pipelines, and programmatic changes
2. **Use file editing for complex content**: When you need fine control over formatting
3. **Organize by domain**: Put domain-specific rules in `domains/name/` directories
4. **Validate after changes**: Run `ai-rulez validate` to check configuration
5. **Regenerate after changes**: Run `ai-rulez generate` to create tool-specific outputs
6. **Commit both sources and outputs**: Version control both `.ai-rulez/` and generated files

### Programmatic Workflow Example

```bash
#!/bin/bash
# Example: Create a new domain with rules

# Create the domain
ai-rulez domain add backend --description "Backend services"

# Add a rule
ai-rulez add rule database-standards --domain backend --priority high

# Add context
ai-rulez add context architecture --domain backend

# Validate
ai-rulez validate

# Generate
ai-rulez generate

# Commit
git add .ai-rulez/ CLAUDE.md .cursor/
git commit -m "chore: add backend domain with database standards"
```

## Best Practices

### Domain Names

Use names that indicate ownership or responsibility:
- `backend`, `frontend`, `mobile` (service boundaries)
- `api`, `database`, `queue` (technical components)
- `auth`, `payments`, `search` (feature areas)

Avoid: `team1`, `team2`, single letters, overly broad names

### Organizing Content

Put content in the domain that owns it. Example:
- `domains/backend/rules/database-standards.md`
- `domains/frontend/rules/accessibility.md`

### Single Responsibility

Each domain should represent one area:

```yaml
Good:
profiles:
  full: [api, frontend, infrastructure]

Bad:
profiles:
  full: [api-with-db, frontend-with-build, infrastructure-and-monitoring]
```

### Document Domain Purposes

Add comments to `config.yaml`:

```yaml
# Domains:
# - backend: Go services, REST APIs
# - frontend: React web app
# - mobile: React Native apps
# - devops: Infrastructure, deployment

profiles:
  full: [backend, frontend, mobile, devops]
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
