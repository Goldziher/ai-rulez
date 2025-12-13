# Includes System

The includes system lets you reuse configurations across multiple projects.

## Overview

The includes system works through configuration inheritance and composition:

1. **Single source of truth**: Define common rules once
2. **Reusable modules**: Share rules, context, and skills across projects
3. **Composable**: Mix and match includes to create project-specific configurations
4. **Version-controlled**: Track changes to shared configurations

## How Includes Work

Includes allow one `.ai-rulez/` configuration to inherit content from other configurations. This is useful for:

- **Organization-wide standards**: Share coding standards across all projects
- **Framework-specific rules**: Reuse React, Go, or Python framework guidelines
- **Security policies**: Enforce consistent security practices
- **Team conventions**: Share team-specific workflows

## Basic Example

### Creating a Shared Configuration

Create a `.ai-rulez/` directory that others can include:

**`shared-rules/.ai-rulez/config.yaml`:**
```yaml
version: "3.0"
name: "shared-rules"
description: "Organization-wide AI rules"

presets: []

profiles:
  default: []
```

**`shared-rules/.ai-rulez/rules/security.md`:**
```markdown
---
priority: critical
---

# Security Standards

- Always validate user input
- Use parameterized queries
- Never hardcode secrets
- Rotate credentials regularly
```

### Including in Your Project

In your project's `.ai-rulez/config.yaml`, reference the shared rules:

```yaml
version: "3.0"
name: "my-project"

# Include rules from another directory
includes:
  - ../shared-rules/.ai-rulez

presets:
  - claude
  - cursor

profiles:
  default: []
```

Now your project includes all content from the shared configuration.

## Include Paths

Includes can be:

1. **Relative paths**: `../shared-rules/.ai-rulez`, `./team-guidelines/.ai-rulez`
2. **Absolute paths**: `/etc/ai-rulez-standards/.ai-rulez`
3. **Git URLs** (planned): `https://github.com/org/shared-rules/.ai-rulez`

### Examples

**Sibling directory:**
```yaml
includes:
  - ../shared-rules/.ai-rulez
```

**Subdirectory:**
```yaml
includes:
  - ./config/shared/.ai-rulez
```

**Multiple includes:**
```yaml
includes:
  - ../shared-rules/.ai-rulez
  - ../team-guidelines/.ai-rulez
  - ./security-policies/.ai-rulez
```

## Include Priority

When the same file exists in multiple includes and your configuration:

1. **Your configuration** takes precedence (highest priority)
2. **Includes** are merged in order
3. **Later includes** override earlier ones

Example:

```yaml
includes:
  - ../base-rules/.ai-rulez      # Loaded first
  - ../team-rules/.ai-rulez      # Overrides base-rules
  # Your rules/ directory overrides both
```

If multiple includes define `.ai-rulez/rules/security.md`, the last one wins.

## Common Patterns

### Organization-Wide Standards

Create a central repository with baseline rules:

**Repository structure:**
```
org-standards/
└── .ai-rulez/
    ├── config.yaml
    ├── rules/
    │   ├── security.md
    │   ├── code-quality.md
    │   └── git-workflow.md
    └── context/
        └── company-values.md
```

**Each project includes it:**
```yaml
includes:
  - https://github.com/myorg/standards/.ai-rulez
```

### Framework-Specific Rules

Create separate includes for each framework:

```
frameworks/
├── go-backend/
│   └── .ai-rulez/
│       ├── config.yaml
│       └── rules/
│           ├── project-layout.md
│           ├── error-handling.md
│           └── testing.md
├── react-frontend/
│   └── .ai-rulez/
│       └── rules/
│           ├── component-guidelines.md
│           ├── hooks-patterns.md
│           └── styling.md
└── python-ml/
    └── .ai-rulez/
        └── rules/
            ├── numpy-conventions.md
            └── ml-best-practices.md
```

**Your project uses them:**
```yaml
includes:
  - ../../frameworks/go-backend/.ai-rulez
  - ../../frameworks/react-frontend/.ai-rulez

presets:
  - claude
  - cursor

profiles:
  backend:
    - backend
  frontend:
    - frontend
  full:
    - backend
    - frontend
```

### Monorepo with Shared and Team-Specific Rules

**Repository structure:**
```
monorepo/
├── shared-rules/.ai-rulez/     # Used by all teams
├── backend-team/
│   └── .ai-rulez/              # Includes shared + backend-specific
├── frontend-team/
│   └── .ai-rulez/              # Includes shared + frontend-specific
└── mobile-team/
    └── .ai-rulez/              # Includes shared + mobile-specific
```

**`backend-team/.ai-rulez/config.yaml`:**
```yaml
version: "3.0"
name: "backend-api"

includes:
  - ../shared-rules/.ai-rulez

presets:
  - claude
  - cursor

profiles:
  default: []
```

## Multi-Level Hierarchy

Includes can themselves include other includes:

**hierarchy:**
```
org-base/
├── rules/
│   └── security.md
└── .ai-rulez/config.yaml

go-framework/
└── .ai-rulez/config.yaml
    includes:
      - ../org-base/.ai-rulez

my-project/
└── .ai-rulez/config.yaml
    includes:
      - ../go-framework/.ai-rulez
      - ../team-standards/.ai-rulez
```

When you generate from `my-project`, it loads:
1. `org-base` rules (base layer)
2. `go-framework` rules (framework-specific)
3. `team-standards` rules (team-specific)
4. `my-project` rules (project-specific)

## Collision Handling

When includes define the same file:

```
shared-rules/.ai-rulez/rules/testing.md
go-framework/.ai-rulez/rules/testing.md
my-project/.ai-rulez/rules/testing.md
```

Resolution order (last one wins):
1. `shared-rules/rules/testing.md` (loaded first)
2. `go-framework/rules/testing.md` (overrides shared)
3. `my-project/rules/testing.md` (overrides both)

A warning is logged:
```
⚠️  Content collision: rules/testing.md found in multiple includes
    → Using: my-project/.ai-rulez/rules/testing.md
```

## Best Practices

### 1. Organize by Level of Specificity

```
org-wide-rules/           # Applies to everything
team-rules/               # Team-specific
framework-rules/          # Technology-specific
project-rules/            # Project-specific
```

### 2. Use Clear Naming

```
Good:
- org-standards/.ai-rulez
- react-best-practices/.ai-rulez
- backend-security/.ai-rulez

Bad:
- rules/.ai-rulez        (ambiguous)
- base/.ai-rulez          (unclear scope)
```

### 3. Document Includes

Add comments to your config explaining why you're including something:

```yaml
version: "3.0"
name: "my-backend"

includes:
  # Organization-wide coding standards
  - ../org-standards/.ai-rulez

  # Go-specific conventions
  - ../go-guidelines/.ai-rulez

  # Backend team standards
  - ../backend-team/.ai-rulez

presets:
  - claude
  - cursor
```

### 4. Keep Includes Focused

Each include should have a single purpose:

```
Good structure:
security-policies/
  - rules for security

error-handling/
  - rules for error handling

Code-style/
  - rules for code style

Bad structure:
everything/
  - security, errors, style, testing, etc.
```

### 5. Version Your Includes

If includes are in separate repositories, use version control:

```bash
# Tag releases
git tag v1.0.0 org-standards/

# Reference specific versions
includes:
  - https://github.com/org/standards/.ai-rulez@v1.0.0
```

## Troubleshooting

### Include Path Not Found

```bash
# Check the path exists
ls -la ../shared-rules/.ai-rulez/config.yaml

# Try absolute path
includes:
  - /path/to/shared-rules/.ai-rulez
```

### Circular Includes

If `a` includes `b`, and `b` includes `a`:

```bash
# Error: Circular include detected
#   a -> b -> a

# Solution: Restructure to avoid cycles
# Create a base layer that both include from
```

### Conflicting Rules

If two includes define conflicting rules:

```bash
# Use project-level rules to override
# Your .ai-rulez/rules/security.md overrides both includes

# Or change include order to control priority
includes:
  - ../stricter-rules/.ai-rulez    # Load strict rules first
  - ../lenient-rules/.ai-rulez     # Load lenient rules last (wins)
```

### Content Not Merging

```bash
# Verify includes are in correct format
cat .ai-rulez/config.yaml | grep includes

# Check include directory exists
ls -la ../shared-rules/.ai-rulez/

# Validate configuration
ai-rulez validate --verbose
```

## Migration Path

If you're currently using separate configurations:

1. **Extract common rules** into a shared include:
   ```bash
   mkdir -p ../shared-rules/.ai-rulez/rules
   # Move common rules there
   ```

2. **Add include** to your config:
   ```yaml
   includes:
     - ../shared-rules/.ai-rulez
   ```

3. **Regenerate** and test:
   ```bash
   ai-rulez validate
   ai-rulez generate
   ```

4. **Commit** the include:
   ```bash
   git add ../shared-rules/
   git commit -m "chore: extract shared rules"
   ```

## Next Steps

- **[Configuration Reference](configuration.md)**: Advanced config options
- **[Domains & Profiles](domains.md)**: Team organization
- **[Quick Start](quick-start.md)**: Getting started
