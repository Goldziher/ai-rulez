# CLI Reference

All AI-Rulez CLI commands and flags.

## Command Overview

### Core Commands
| Command | Description |
|---------|-------------|
| `ai-rulez init` | Initialize V3 directory-based configuration |
| `ai-rulez generate` | Generate presets for specific profile |
| `ai-rulez validate` | Validate configuration |
| `ai-rulez version` | Show version |
| `ai-rulez mcp` | Start MCP server |

### CRUD Commands (Configuration Management)
| Command | Description |
|---------|-------------|
| `ai-rulez domain add/remove/list` | Manage domains |
| `ai-rulez add rule/context/skill` | Create content files |
| `ai-rulez remove rule/context/skill` | Delete content files |
| `ai-rulez list rules/context/skills` | List content files |
| `ai-rulez include add/remove/list` | Manage external includes |
| `ai-rulez profile add/remove/list` | Manage profiles |
| `ai-rulez profile set-default` | Set default profile |

## CRUD Commands

V3 provides CRUD commands to programmatically modify your `.ai-rulez/` configuration. These commands allow you to create domains, add rules/context/skills, manage includes, and organize profiles.

### Domain Management

#### `ai-rulez domain add <name> [flags]`

Create a new domain with subdirectories for rules, context, and skills.

**Syntax:**
```bash
ai-rulez domain add <name> [flags]
```

**Arguments:**
- `<name>` (required): Domain name. Alphanumeric and underscores, 1-50 characters.

**Flags:**
- `--description <text>` (optional): Description of the domain
- `--verbose` (optional): Show detailed output

**Examples:**

Create a backend domain:
```bash
ai-rulez domain add backend
```

Create a domain with description:
```bash
ai-rulez domain add frontend --description "React web application"
```

#### `ai-rulez domain remove <name> [flags]`

Delete a domain and all its contents.

**Syntax:**
```bash
ai-rulez domain remove <name> [flags]
```

**Arguments:**
- `<name>` (required): Domain name to delete

**Flags:**
- `--force` (optional): Skip confirmation prompt

**Examples:**

Remove a domain (with confirmation):
```bash
ai-rulez domain remove backend
```

Remove without confirmation:
```bash
ai-rulez domain remove backend --force
```

#### `ai-rulez domain list [flags]`

List all domains in the `.ai-rulez/` directory.

**Syntax:**
```bash
ai-rulez domain list [flags]
```

**Flags:**
- `--json` (optional): Output as JSON
- `--verbose` (optional): Show file counts per domain

**Examples:**

List domains:
```bash
ai-rulez domain list
```

List as JSON:
```bash
ai-rulez domain list --json
```

### Content Management

Rules, context, and skills can be added to the root (always included) or to specific domains (profile-dependent).

#### `ai-rulez add rule <name> [flags]`

Create a new rule file with optional YAML frontmatter.

**Syntax:**
```bash
ai-rulez add rule <name> [flags]
```

**Arguments:**
- `<name>` (required): Rule filename without `.md` extension

**Flags:**
- `--domain <name>` (optional): Domain name. If not specified, creates in root rules directory
- `--priority <level>` (optional): Priority: critical, high, medium, low. Default: medium
- `--targets <list>` (optional): Comma-separated list of target providers (claude, cursor, etc.)
- `--file <path>` (optional): Read content from a file instead of stdin
- `--verbose` (optional): Show detailed output

**Examples:**

Create a root rule:
```bash
ai-rulez add rule code-quality
```

Create a domain-specific rule:
```bash
ai-rulez add rule database-standards --domain backend --priority high
```

Create with specific targets:
```bash
ai-rulez add rule performance --targets claude,cursor --priority high
```

#### `ai-rulez add context <name> [flags]`

Create a new context file (documentation/reference material).

**Syntax:**
```bash
ai-rulez add context <name> [flags]
```

**Arguments:**
- `<name>` (required): Context filename without `.md` extension

**Flags:**
- `--domain <name>` (optional): Domain name. If not specified, creates in root context directory
- `--priority <level>` (optional): Priority: critical, high, medium, low. Default: medium
- `--targets <list>` (optional): Comma-separated list of target providers
- `--file <path>` (optional): Read content from a file
- `--verbose` (optional): Show detailed output

**Examples:**

Create root context:
```bash
ai-rulez add context architecture
```

Create domain context:
```bash
ai-rulez add context database-design --domain backend
```

#### `ai-rulez add skill <name> [flags]`

Create a new skill file (AI prompt/expert definition).

**Syntax:**
```bash
ai-rulez add skill <name> [flags]
```

**Arguments:**
- `<name>` (required): Skill filename without `.md` extension

**Flags:**
- `--domain <name>` (optional): Domain name. If not specified, creates in root skills directory
- `--priority <level>` (optional): Priority: critical, high, medium, low. Default: medium
- `--targets <list>` (optional): Comma-separated list of target providers
- `--file <path>` (optional): Read content from a file
- `--verbose` (optional): Show detailed output

**Examples:**

Create a root skill:
```bash
ai-rulez add skill code-reviewer
```

Create a domain-specific skill:
```bash
ai-rulez add skill performance-optimizer --domain backend --priority high
```

#### `ai-rulez remove rule <name> [flags]`

Delete a rule file.

**Syntax:**
```bash
ai-rulez remove rule <name> [flags]
```

**Arguments:**
- `<name>` (required): Rule filename without `.md` extension

**Flags:**
- `--domain <name>` (optional): Domain name
- `--force` (optional): Skip confirmation

**Examples:**

Remove a root rule:
```bash
ai-rulez remove rule code-quality
```

Remove a domain rule:
```bash
ai-rulez remove rule database-standards --domain backend --force
```

#### `ai-rulez remove context <name> [flags]`

Delete a context file.

**Syntax:**
```bash
ai-rulez remove context <name> [flags]
```

**Arguments:**
- `<name>` (required): Context filename without `.md` extension

**Flags:**
- `--domain <name>` (optional): Domain name
- `--force` (optional): Skip confirmation

**Examples:**

```bash
ai-rulez remove context architecture
ai-rulez remove context backend-design --domain backend --force
```

#### `ai-rulez remove skill <name> [flags]`

Delete a skill file.

**Syntax:**
```bash
ai-rulez remove skill <name> [flags]
```

**Arguments:**
- `<name>` (required): Skill filename without `.md` extension

**Flags:**
- `--domain <name>` (optional): Domain name
- `--force` (optional): Skip confirmation

**Examples:**

```bash
ai-rulez remove skill code-reviewer
ai-rulez remove skill performance-optimizer --domain backend --force
```

#### `ai-rulez list rules [flags]`

List all rule files.

**Syntax:**
```bash
ai-rulez list rules [flags]
```

**Flags:**
- `--domain <name>` (optional): List rules in specific domain only
- `--json` (optional): Output as JSON
- `--verbose` (optional): Show file details

**Examples:**

List all rules:
```bash
ai-rulez list rules
```

List domain rules:
```bash
ai-rulez list rules --domain backend
```

#### `ai-rulez list context [flags]`

List all context files.

**Syntax:**
```bash
ai-rulez list context [flags]
```

**Flags:**
- `--domain <name>` (optional): List context in specific domain only
- `--json` (optional): Output as JSON
- `--verbose` (optional): Show file details

**Examples:**

```bash
ai-rulez list context
ai-rulez list context --domain backend
```

#### `ai-rulez list skills [flags]`

List all skill files.

**Syntax:**
```bash
ai-rulez list skills [flags]
```

**Flags:**
- `--domain <name>` (optional): List skills in specific domain only
- `--json` (optional): Output as JSON
- `--verbose` (optional): Show file details

**Examples:**

```bash
ai-rulez list skills
ai-rulez list skills --domain backend
```

### Include Management

Manage external rule sources (git repositories or local packages).

#### `ai-rulez include add <name> <source> [flags]`

Add a new include source to the configuration.

**Syntax:**
```bash
ai-rulez include add <name> <source> [flags]
```

**Arguments:**
- `<name>` (required): Unique identifier for this include
- `<source>` (required): Git URL (e.g., `https://github.com/org/repo`) or local path (e.g., `./packages/shared`)

**Flags:**
- `--path <dir>` (optional): Path within git repository where `.ai-rulez/` content is located
- `--ref <branch>` (optional): Git reference (branch, tag, commit). Default: main
- `--include <types>` (optional): Comma-separated content types: rules,context,skills,mcp
- `--merge-strategy <strategy>` (optional): Merge strategy: default, override, append
- `--install-to <path>` (optional): Installation target path in `.ai-rulez/`

**Examples:**

Add a git-based include:
```bash
ai-rulez include add corporate-rules https://github.com/myorg/shared-rules
```

Add with custom path and ref:
```bash
ai-rulez include add shared-patterns https://github.com/myorg/repo --path .ai-rulez --ref develop
```

Add local include:
```bash
ai-rulez include add backend-package ./packages/backend
```

#### `ai-rulez include remove <name> [flags]`

Remove an include source.

**Syntax:**
```bash
ai-rulez include remove <name> [flags]
```

**Arguments:**
- `<name>` (required): Include name to remove

**Flags:**
- `--force` (optional): Skip confirmation

**Examples:**

```bash
ai-rulez include remove corporate-rules
ai-rulez include remove shared-patterns --force
```

#### `ai-rulez include list [flags]`

List all include sources.

**Syntax:**
```bash
ai-rulez include list [flags]
```

**Flags:**
- `--json` (optional): Output as JSON
- `--verbose` (optional): Show detailed configuration

**Examples:**

```bash
ai-rulez include list
ai-rulez include list --json
```

### Profile Management

Organize domains into named profiles for targeted generation.

#### `ai-rulez profile add <name> <domains...> [flags]`

Create a new profile.

**Syntax:**
```bash
ai-rulez profile add <name> <domains...> [flags]
```

**Arguments:**
- `<name>` (required): Profile name
- `<domains...>` (required): Space-separated list of domain names to include

**Flags:**
- `--set-default` (optional): Set this as the default profile
- `--verbose` (optional): Show detailed output

**Examples:**

Create a backend profile:
```bash
ai-rulez profile add backend backend qa
```

Create and set as default:
```bash
ai-rulez profile add full backend frontend qa --set-default
```

#### `ai-rulez profile remove <name> [flags]`

Delete a profile.

**Syntax:**
```bash
ai-rulez profile remove <name> [flags]
```

**Arguments:**
- `<name>` (required): Profile name to remove

**Flags:**
- `--force` (optional): Skip confirmation

**Examples:**

```bash
ai-rulez profile remove staging
ai-rulez profile remove development --force
```

#### `ai-rulez profile set-default <name> [flags]`

Set a profile as the default for generation.

**Syntax:**
```bash
ai-rulez profile set-default <name> [flags]
```

**Arguments:**
- `<name>` (required): Profile name to set as default

**Examples:**

```bash
ai-rulez profile set-default full
```

#### `ai-rulez profile list [flags]`

List all profiles.

**Syntax:**
```bash
ai-rulez profile list [flags]
```

**Flags:**
- `--json` (optional): Output as JSON
- `--verbose` (optional): Show domain contents

**Examples:**

```bash
ai-rulez profile list
ai-rulez profile list --json
```

---

## Initialization Command

### `ai-rulez init [project-name] --v3`

Initialize a new V3 directory-based configuration.

**Syntax:**
```bash
ai-rulez init [project-name] --v3 [flags]
```

**Arguments:**
- `[project-name]` (optional): The project name. If not provided, prompted interactively.

**V3-specific Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--v3` | boolean | false | Use V3 directory-based configuration |
| `--format` | string | `yaml` | Configuration format: `yaml` or `json` |
| `--domains` | string | (none) | Comma-separated list of domain names to create |
| `--skip-content` | boolean | false | Skip creating example content files |

**General Flags:**

| Flag | Type | Description |
|------|------|-------------|
| `--setup-hooks` | boolean | Configure Git hooks after initialization |
| `--verbose` | boolean | Enable verbose output |
| `--debug` | boolean | Enable debug output |

**Examples:**

Basic V3 initialization:
```bash
ai-rulez init "my-project" --v3
```

V3 with JSON format:
```bash
ai-rulez init "my-project" --v3 --format json
```

V3 with multiple domains:
```bash
ai-rulez init "my-project" --v3 --domains "backend,frontend,qa"
```

V3 with example content skipped:
```bash
ai-rulez init "my-project" --v3 --skip-content
```

## Generate Command

### `ai-rulez generate [config-path]`

Generate AI assistant rule files from configuration.

**Syntax:**
```bash
ai-rulez generate [config-path] [flags]
```

**Arguments:**
- `[config-path]` (optional): Path to configuration file or directory. If not provided, auto-detected.

**V3-specific Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--profile` | string | (from config) | Profile to generate (V3 only) |

**General Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | boolean | false | Show what would be generated without writing files |
| `--update-gitignore` | boolean | (from config) | Update `.gitignore` files with generated outputs |
| `--recursive` / `-r` | boolean | false | Find and process configs recursively |
| `--token` | string | (from env) | Git access token for private repositories (or use `AI_RULEZ_GIT_TOKEN` env var) |

**Examples:**

Generate with default profile:
```bash
ai-rulez generate
```

Generate specific profile:
```bash
ai-rulez generate --profile backend
```

Dry-run to preview generation:
```bash
ai-rulez generate --dry-run --profile backend
```

Generate and update .gitignore:
```bash
ai-rulez generate --profile full --update-gitignore
```

Generate recursively in monorepo:
```bash
ai-rulez generate --recursive
```

Generate with private repository authentication:
```bash
# Using environment variable (recommended)
export AI_RULEZ_GIT_TOKEN="ghp_your_github_token_here"
ai-rulez generate

# Using CLI flag
ai-rulez generate --token "ghp_your_github_token_here"
```

### Profile Selection

When using V3 configuration:

1. **No `--profile` flag**: Uses default profile from config
2. **`--profile backend`**: Generates content for the `backend` profile
3. **Invalid profile**: Shows error and lists available profiles

Example with profile hierarchy:

```yaml
# config.yaml
default: "full"
profiles:
  full:
    - backend
    - frontend
    - qa
  backend:
    - backend
  frontend:
    - frontend
```

```bash
# Uses "full" profile (default)
ai-rulez generate

# Uses "backend" profile only
ai-rulez generate --profile backend

# Uses "frontend" profile only
ai-rulez generate --profile frontend
```

## Validation Command

### `ai-rulez validate [config-path]`

Validate configuration without generating files.

**Syntax:**
```bash
ai-rulez validate [config-path] [flags]
```

**Arguments:**
- `[config-path]` (optional): Path to configuration file. If not provided, auto-detected.

**Flags:**

| Flag | Type | Description |
|------|------|-------------|
| `--verbose` | boolean | Enable verbose output |
| `--debug` | boolean | Enable debug output |

**Examples:**

Validate current configuration:
```bash
ai-rulez validate
```

Validate specific config file:
```bash
ai-rulez validate .ai-rulez/config.yaml
```

With verbose output:
```bash
ai-rulez validate --verbose
```

### What Gets Validated

- `version` is exactly `"3.0"`
- `name` is present and non-empty
- All preset names are valid
- Referenced domains exist in filesystem
- Profile definitions reference valid domains
- File paths are accessible

## Version Command

### `ai-rulez version`

Show the current version and build information.

```bash
ai-rulez version
```

## MCP Server

### `ai-rulez mcp`

Starts the Model Context Protocol (MCP) server to allow AI assistants to programmatically interact with your configuration.

```bash
ai-rulez mcp
```

See the [MCP Server Documentation](mcp-server.md) for more details.

## Global Flags

These flags work with all commands:

| Flag | Type | Description |
|------|------|-------------|
| `--config` | string | Config file path (auto-discovered if not specified) |
| `--token` | string | Git access token for private repositories (or use `AI_RULEZ_GIT_TOKEN` env var) |
| `--verbose` | boolean | Enable verbose output |
| `--debug` | boolean | Enable debug output |
| `--quiet` / `-q` | boolean | Suppress progress bars and non-essential output |
| `--help` / `-h` | boolean | Show help for a command |

**Examples:**

Generate with debug output:
```bash
ai-rulez generate --debug
```

Quiet mode (minimal output):
```bash
ai-rulez generate --quiet
```

Show help for init:
```bash
ai-rulez init --help
```

## Configuration Detection

The CLI auto-detects configuration in the following order:

1. **Explicit path**: Via `--config` flag or command argument
2. **V3 format**: `.ai-rulez/` directory in current directory
3. **V2 format**: `ai-rulez.yaml` or `ai-rulez.yml` in current directory
4. **Error**: No configuration found

Example detection flow:

```bash
cd /path/to/project

# Detects .ai-rulez/ if it exists (V3)
ai-rulez generate

# Use explicit path
ai-rulez generate .ai-rulez/config.yaml

# Specify config via flag
ai-rulez generate --config ./custom-config.yaml
```

## Exit Codes

The CLI uses standard exit codes:

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error (config not found, validation failed, etc.) |
| 2 | Command syntax error (invalid flags, arguments) |

## Output Examples

### Initialize Output

```
✅ Created .ai-rulez/ directory structure

Directory structure:
  .ai-rulez/
  ├── config.yaml
  ├── rules/         # Base rules (always included)
  ├── context/       # Base context (always included)
  ├── skills/        # Base skills (always included)
  ├── agents/        # Base agents (always included)
  └── domains/       # Domain-specific content

Example content created:
  - rules/code-quality.md
  - context/architecture.md
  - skills/code-reviewer/SKILL.md

Example MCP servers created:
  - mcp.yaml         # Root MCP servers (ai-rulez + GitHub examples)

Domain directories created:
  - domains/backend/
  - domains/frontend/
  - domains/qa/

Next steps:
  1. Edit .ai-rulez/config.yaml to customize presets and profiles
  2. Add your rules, context, and skills to the appropriate directories
  3. Run 'ai-rulez generate' to create tool-specific outputs
```

### Generate Output

```
✅ Generated 3 file(s) successfully
  - CLAUDE.md
  - .cursorrules
  - docs/AI_GUIDE.md
```

### Validation Output

```
✅ Configuration is valid
  Project: my-project
  Version: 3.0
  Presets: 3
  Domains: 3
  Profiles: 2
```

### Validation Error

```
❌ Configuration validation failed

errors:
  - Profile "backend" references undefined domain "backend"
  - Preset "claude" not found

Run 'ai-rulez validate --verbose' for details
```

## Common CRUD Workflows

### Creating a Domain with Rules

```bash
# Create a domain
ai-rulez domain add backend --description "Backend services"

# Add rules to the domain
ai-rulez add rule database-standards --domain backend --priority high
ai-rulez add rule api-design --domain backend --priority high

# Add context
ai-rulez add context architecture --domain backend

# Validate and generate
ai-rulez validate
ai-rulez generate
```

### Setting Up Team-Based Profiles

```bash
# Create domains for each team
ai-rulez domain add backend
ai-rulez domain add frontend
ai-rulez domain add qa

# Create team-specific profiles
ai-rulez profile add full backend frontend qa --set-default
ai-rulez profile add backend backend qa
ai-rulez profile add frontend frontend qa

# Generate for a specific profile
ai-rulez generate --profile backend
```

### Adding External Rules from Git

```bash
# Add corporate rules include
ai-rulez include add corporate-rules https://github.com/myorg/shared-rules --ref main

# Verify include was added
ai-rulez include list

# Validate with included content
ai-rulez validate

# Regenerate with included content
ai-rulez generate
```

### Managing Content Across Domains

```bash
# Add shared rule to root (always included)
ai-rulez add rule code-style --priority high

# Add domain-specific rule
ai-rulez add rule database-standards --domain backend --priority high
ai-rulez add rule accessibility --domain frontend --priority high

# List all rules
ai-rulez list rules

# List domain-specific rules
ai-rulez list rules --domain backend
ai-rulez list rules --domain frontend

# Remove a rule
ai-rulez remove rule old-guideline --force
```

---

## Common Workflows

### Set Up a New V3 Project

```bash
# Initialize with domains
ai-rulez init "my-project" --v3 --domains "backend,frontend,qa"

# Review generated structure
ls -la .ai-rulez/

# Create/edit content files
# (edit .ai-rulez/rules/, .ai-rulez/context/, etc.)

# Validate configuration
ai-rulez validate

# Generate outputs
ai-rulez generate
```

### Generate Multiple Profiles

```bash
# Generate full profile
ai-rulez generate --profile full

# Generate backend-only profile
ai-rulez generate --profile backend

# Generate frontend-only profile
ai-rulez generate --profile frontend
```

### Update AI Configuration After Changes

```bash
# Edit your content
vim .ai-rulez/rules/my-rule.md

# Validate it's still correct
ai-rulez validate

# Regenerate outputs
ai-rulez generate

# Commit changes
git add .ai-rulez/ CLAUDE.md .cursor/ GEMINI.md
git commit -m "docs: update AI assistant guidelines"
```

### CI/CD Integration

```bash
#!/bin/bash
# Simple CI/CD script

# Validate configuration
ai-rulez validate || exit 1

# Generate all outputs
ai-rulez generate || exit 1

# Check for uncommitted changes
if ! git diff --quiet CLAUDE.md .cursor/ GEMINI.md; then
  echo "Generated files are out of sync"
  echo "Run: ai-rulez generate"
  exit 1
fi
```

## Troubleshooting

### Command not found

```bash
# Make sure AI-Rulez is installed
which ai-rulez

# Or check version
ai-rulez version
```

### Configuration not found

```bash
# Check current directory
ls -la .ai-rulez/
ls -la ai-rulez.yaml

# Or specify explicitly
ai-rulez generate --config /path/to/config.yaml
```

### Invalid profile name

```bash
# List available profiles
ai-rulez validate --verbose

# Use a valid profile name
ai-rulez generate --profile backend
```

### Generated files not updated

```bash
# Check if files were actually generated
ai-rulez generate --dry-run

# Force regeneration
rm CLAUDE.md .cursor/rules/*
ai-rulez generate
```

## Help and Documentation

Get help for any command:

```bash
ai-rulez --help
ai-rulez init --help
ai-rulez generate --help
ai-rulez validate --help
```

For more detailed documentation:
- **[Configuration Reference](configuration.md)**: Config options
- **[Quick Start](quick-start.md)**: Getting started
- **[Domains & Profiles](domains.md)**: Team organization
