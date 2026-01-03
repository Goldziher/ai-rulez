# Includes System

Reuse configurations across multiple projects through inheritance and composition.

## Overview

Includes work through configuration inheritance:

1. Define common rules once in a shared configuration
2. Share rules, context, and skills across projects
3. Mix and match includes to create project-specific configurations
4. Track changes through version control

## How Includes Work

Includes allow one `.ai-rulez/` configuration to inherit content from other configurations. Use for:

- Organization-wide coding standards
- Framework-specific guidelines (React, Go, Python)
- Consistent security policies
- Team-specific workflows

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

1. Relative paths: `../shared-rules/.ai-rulez`, `./team-guidelines/.ai-rulez`
2. Absolute paths: `/etc/ai-rulez-standards/.ai-rulez`
3. Git URLs: `https://github.com/org/shared-rules.git`, `git@github.com:org/shared-rules.git`

### Examples

**Sibling directory:**
```yaml
includes:
  - name: shared-rules
    source: ../shared-rules
    include:
      - rules
      - context
    merge_strategy: local-override
```

**Subdirectory:**
```yaml
includes:
  - name: team-config
    source: ./config/shared
    include:
      - rules
      - skills
    merge_strategy: local-override
```

**Git repository (HTTPS):**
```yaml
includes:
  - name: org-standards
    source: https://github.com/myorg/shared-rules.git
    ref: main
    include:
      - rules
      - context
      - skills
      - agents
    merge_strategy: local-override
```

**Git repository (SSH):**
```yaml
includes:
  - name: company-policies
    source: git@github.com:company/ai-rulez.git
    ref: v1.2.3
    include:
      - rules
      - context
    merge_strategy: local-override
```

**Multiple includes:**
```yaml
includes:
  # Local relative path
  - name: team-guidelines
    source: ../team-guidelines
    include:
      - rules
      - context
    merge_strategy: local-override

  # Git repository
  - name: org-standards
    source: git@gitlab.com:org/standards.git
    ref: main
    include:
      - rules
      - skills
    merge_strategy: local-override

  # Another local path
  - name: security-policies
    source: ./security-policies
    include:
      - rules
    merge_strategy: local-override
```

### Supported Git URL Formats

- **HTTPS:** `https://github.com/owner/repo.git`
- **SSH:** `git@github.com:owner/repo.git`
- **SSH protocol:** `ssh://git@github.com/owner/repo.git`
- **GitLab:** `https://gitlab.com/owner/repo.git`, `git@gitlab.com:owner/repo.git`
- **Self-hosted GitLab:** `git@git.example.com:owner/repo.git`, `https://git.example.com/owner/repo.git`

### SSH Cloning for Private Repositories

For private repositories that require SSH authentication, ai-rulez automatically uses `git clone` when it detects SSH URLs (`git@...` or `ssh://...`). This leverages your existing SSH key configuration.

**Benefits of SSH cloning:**
- Works with private repositories without needing access tokens
- Uses your configured SSH keys and agent
- Supports self-hosted Git servers (GitLab, Gitea, Gogs, etc.)
- Ideal for local development and multi-repo setups

**Example with SSH:**
```yaml
includes:
  - name: private-rules
    source: git@git.example.com:company/ai-rulez.git
    ref: main
    include:
      - rules
      - context
    merge_strategy: local-override
```

**Requirements:**
- Git must be installed and available in your PATH
- SSH keys must be configured for the git host
- SSH agent should be running (for passphrase-protected keys)

### Repository Structure Support

ai-rulez supports two repository structures for includes:

1. **Standard structure** (recommended): Repository contains a `.ai-rulez/` subdirectory with the configuration
   ```
   my-repo/
   ├── .ai-rulez/
   │   ├── config.yaml
   │   ├── rules/
   │   ├── context/
   │   └── skills/
   └── other files...
   ```

2. **Root-level structure**: Repository root IS the ai-rulez structure (no nested `.ai-rulez/` directory)
   ```
   my-repo/
   ├── config.yaml
   ├── rules/
   ├── context/
   └── skills/
   ```

ai-rulez automatically detects which structure your repository uses and handles it accordingly.

### Private Repository Authentication (HTTPS)

When working with private Git repositories in includes, you can authenticate using an access token.

#### Using Environment Variable (Recommended)

Set the `AI_RULEZ_GIT_TOKEN` environment variable with your access token:

```bash
export AI_RULEZ_GIT_TOKEN="ghp_your_github_token_here"
ai-rulez generate
```

This is the recommended approach for CI/CD environments and automation scripts.

#### Using CLI Flag

Pass the token directly via the `--token` flag:

```bash
ai-rulez generate --token "ghp_your_github_token_here"
```

#### Creating Access Tokens

**GitHub:**
1. Go to Settings → Developer settings → Personal access tokens
2. Click "Generate new token (classic)"
3. Select scopes:
   - `repo` - Required for accessing private repositories
4. Generate and copy the token

**GitLab:**
1. Go to User Settings → Access Tokens
2. Click "Add new token"
3. Select scopes:
   - `read_repository` - Required for reading private repositories
4. Create token and copy it

**Other Git Hosts:**

Most Git hosting platforms that support Bearer token authentication will work with ai-rulez. The token is sent as a Bearer token in the Authorization header when fetching repository archives.

#### Security Best Practices

- **Never commit tokens** to your repository or configuration files
- **Use environment variables** in CI/CD pipelines (GitHub Actions secrets, GitLab CI/CD variables, etc.)
- **Store tokens securely** using secret management systems (AWS Secrets Manager, HashiCorp Vault, etc.)
- **Rotate tokens regularly** to limit exposure from potential leaks
- **Use minimal permissions** - only grant the token access to what's needed (read-only repository access)
- **Use organization-level tokens** when possible to manage access centrally

#### Example: CI/CD Integration

**GitHub Actions:**
```yaml
name: Generate AI Rules
on: [push]
jobs:
  generate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Generate
        env:
          AI_RULEZ_GIT_TOKEN: ${{ secrets.GIT_TOKEN }}
        run: npx ai-rulez@latest generate
```

**GitLab CI:**
```yaml
generate:
  script:
    - export AI_RULEZ_GIT_TOKEN="$CI_JOB_TOKEN"
    - ai-rulez generate
```

### Include Options

- **`name`**: Unique identifier for the include
- **`source`**: Path or Git URL to the configuration
- **`ref`**: (Git only) Branch, tag, or commit SHA (default: `main`)
- **`include`**: List of content types to fetch: `rules`, `context`, `skills`, `agents`
- **`merge_strategy`**: How to handle conflicts:
  - `local-override`: Local content takes precedence (default)
  - `remote-override`: Remote content takes precedence

## Include Priority

When the same file exists in multiple includes and your configuration:

1. Your configuration takes precedence (highest priority)
2. Includes are merged in order
3. Later includes override earlier ones

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

### Organize by Specificity

```
org-wide-rules/           Applies to everything
team-rules/               Team-specific
framework-rules/          Technology-specific
project-rules/            Project-specific
```

### Use Clear Naming

Good:
- `org-standards/.ai-rulez`
- `react-best-practices/.ai-rulez`
- `backend-security/.ai-rulez`

Bad:
- `rules/.ai-rulez` (ambiguous)
- `base/.ai-rulez` (unclear scope)

### Document Includes

Add comments to your config explaining why includes are needed:

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

### Keep Includes Focused

Each include should have a single purpose:

```
Good:
security-policies/       Rules for security
error-handling/          Rules for error handling
code-style/              Rules for code style

Bad:
everything/              Security, errors, style, testing, etc.
```

### Version Your Includes

Tag releases and reference specific versions:

```bash
git tag v1.0.0 org-standards/

includes:
  - https://github.com/org/standards/.ai-rulez@v1.0.0
```

## Troubleshooting

### Include Path Not Found

```bash
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

Use project-level rules to override, or change include order:

```yaml
includes:
  - ../stricter-rules/.ai-rulez    # Load strict rules first
  - ../lenient-rules/.ai-rulez     # Load lenient rules last (wins)
```

Your `.ai-rulez/rules/security.md` overrides both includes.

### Content Not Merging

```bash
cat .ai-rulez/config.yaml | grep includes
ls -la ../shared-rules/.ai-rulez/
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
