# CLI Commands

## Basic Commands

### init

Create a new project configuration.

```bash
# Simple start - creates Claude configuration
ai-rulez init "My Project"
```

```bash
# Popular AI platforms  
ai-rulez init "My Project" --popular
```

```bash
# Specific platform
ai-rulez init "My Project" --preset cursor
```

### generate

Generate AI assistant files.

```bash
# Generate all configured outputs
ai-rulez generate
```

```bash
# Preview without writing files
ai-rulez generate --dry-run
```

### validate

Check configuration syntax.

```bash
ai-rulez validate
```

### list

Display configuration elements.

```bash
# List all rules
ai-rulez list rules

# List with details
ai-rulez list rules --detailed

# Filter by name
ai-rulez list rules --filter "testing"

# JSON output
ai-rulez list rules --json
```

Available list commands: `rules`, `sections`, `agents`, `outputs`

## Advanced Options

### Provider Selection

```bash
# All supported platforms
ai-rulez init "My Project" --all

# Specific combinations
ai-rulez init "My Project" --claude --cursor --copilot

# Available platforms:
--claude --cursor --windsurf --copilot --gemini --amp --codex --cline --continue-dev
```

### AI Agent Support

```bash
# Use AI to generate configuration
ai-rulez init "My Project" --use-agent claude

# Skip AI assistance  
ai-rulez init "My Project" --no-agent

# List available AI agents
ai-rulez init --list-agents
```

## generate

Generate AI assistant files from configuration.

```bash
ai-rulez generate [flags]
```

### Examples

```bash
# Generate all outputs
ai-rulez generate

# Use specific config file
ai-rulez generate --config custom.yaml

# Dry run (preview without writing)
ai-rulez generate --dry-run

# Generate recursively (find all config files)
ai-rulez generate --recursive
```

## validate

Validate configuration syntax and structure.

```bash
ai-rulez validate [flags]
```

### Examples

```bash
# Validate current directory config
ai-rulez validate

# Validate specific file
ai-rulez validate --config custom.yaml

# Validate recursively
ai-rulez validate --recursive
```

## Configuration Management

Manage your configuration without editing files.

### Managing Rules

```bash
# Add a simple rule
ai-rulez add rule "testing" --content "Write unit tests"

# Add rule with priority
ai-rulez add rule "security" --content "Validate all inputs" --priority 10

# Update rule content
ai-rulez update rule "testing" --content "Write comprehensive unit tests" 

# Remove rule
ai-rulez delete rule "testing"
```

### Managing Outputs

```bash
# Add output for new AI platform
ai-rulez add output --path "GEMINI.md"

# Add directory output
ai-rulez add output --path ".windsurf/" --type "rule" --naming-scheme "rules.md"

# Remove output
ai-rulez delete output --path "GEMINI.md"
```

### Advanced: Sections & Agents

```bash
# Add documentation section
ai-rulez add section "Architecture" --content "Clean architecture with DDD"

# Add Claude agent
ai-rulez add agent "reviewer" \
  --description "Code reviewer" \
  --system-prompt "You are a senior developer" \
  --tools "read,edit"
```

## mcp

Start MCP server for AI assistant integration.

```bash
ai-rulez mcp [flags]
```

### Examples

```bash
# Start MCP server (stdio transport)
ai-rulez mcp

# Use custom config
ai-rulez mcp --config custom.yaml

# Enable debug logging
ai-rulez mcp --debug
```

See [MCP Server documentation](mcp-server.md) for integration details.

## Global Flags

```bash
--config string    Config file path (default: auto-detect)
--debug           Enable debug output with detailed logging
--verbose         Enable verbose output for operations  
--quiet, -q       Suppress progress bars and non-essential output
--help, -h        Show help for any command
--version         Show version number
```

## Additional Commands

### completion

Generate shell completion scripts:

```bash
# Bash
ai-rulez completion bash

# Zsh  
ai-rulez completion zsh

# Fish
ai-rulez completion fish

# PowerShell
ai-rulez completion powershell
```

### help

Get help for any command:

```bash
ai-rulez help
ai-rulez help init
ai-rulez help add rule
```

## Configuration File Detection

ai-rulez looks for configuration files in this order:

1. `--config` flag value
2. `ai_rulez.yaml` 
3. `ai-rulez.yaml`
4. `.ai_rulez.yaml`
5. `.ai-rulez.yaml`
6. Same files with `.yml` extension

Searches current directory, then parent directories recursively.