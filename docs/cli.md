# CLI Reference

Complete reference for all AI-Rulez V3 CLI commands.

## Command Overview

| Command | Description |
|---------|-------------|
| `ai-rulez init` | Initialize V3 directory-based configuration |
| `ai-rulez generate` | Generate presets for specific profile |
| `ai-rulez validate` | Validate configuration |
| `ai-rulez version` | Show version |
| `ai-rulez mcp` | Start MCP server |

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
  └── domains/       # Domain-specific content

Example content created:
  - rules/code-quality.md
  - context/architecture.md
  - skills/code-reviewer/SKILL.md

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
