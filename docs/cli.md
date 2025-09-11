# CLI Reference

All the `ai-rulez` commands you need to know.

## Core Commands

The essentials for day-to-day use.

### `init`

Create a new `ai-rulez.yml` file in the current directory with intelligent project analysis and optional AI assistance.

```bash
# Quick start with popular AI tools
ai-rulez init "My Project" --popular

# Use a specific preset for targeted tools
ai-rulez init "My Project" --preset claude

# Get AI-powered configuration generation
ai-rulez init "My Project" --preset popular --use-agent claude
```

The `init` command is your starting point for any project. It creates a comprehensive configuration that's tailored to your specific codebase, with optional AI assistance to generate project-specific rules and documentation.

#### How It Works

The init process happens in phases:

1. **Project Analysis** - Automatically detects:
   - Programming languages and frameworks (Go, TypeScript, Python, React, etc.)
   - Build tools and package managers (npm, go.mod, poetry, etc.)  
   - Project structure patterns (monorepo, microservices, etc.)
   - Existing documentation and testing setup

2. **Template Generation** - Creates a base configuration with:
   - Metadata based on your project structure
   - Appropriate outputs for selected AI tools
   - Commented examples of advanced features

3. **AI Enhancement** (optional) - If you specify `--use-agent`, the AI will:
   - Generate project-specific rules based on your codebase
   - Create documentation sections describing your architecture
   - Define coding standards relevant to your tech stack
   - Set up specialized agents for your workflow

#### Basic Usage

**Simple initialization:**
```bash
ai-rulez init "My Awesome Project"
```

**With popular AI tools:**
```bash
ai-rulez init "My Awesome Project" --popular
```

**Specific AI tool:**
```bash
ai-rulez init "My Project" --preset claude --with-agents
```

#### Advanced Usage

**AI-powered generation:**
```bash
# Let Claude analyze your project and generate comprehensive rules
ai-rulez init "My Project" --preset popular --use-agent claude

# Use Gemini for configuration generation
ai-rulez init "My Project" --preset gemini --use-agent gemini

# Skip confirmation prompts for automation
ai-rulez init "My Project" --popular --use-agent claude --yes
```

**Custom provider combinations:**
```bash
# Multiple specific providers
ai-rulez init "My Project" --claude --cursor --windsurf

# All supported providers
ai-rulez init "My Project" --all

# Include specialized features
ai-rulez init "My Project" --claude --with-agents --with-sections
```

#### Complete Options

| Option | Description |
|--------|-------------|
| `--popular` | Enable Claude, Cursor, Windsurf, Copilot, and Gemini |
| `--preset <name>` | Use a specific preset (claude, cursor, windsurf, copilot, gemini, amp, cline, continue-dev, codex, opencode) |
| `--all` | Enable all supported AI tools |
| `--claude` | Include Claude configuration (.CLAUDE.md) |
| `--cursor` | Include Cursor configuration (.cursor/rules/) |
| `--windsurf` | Include Windsurf configuration (.windsurf/rules.md) |
| `--copilot` | Include GitHub Copilot configuration (.github/copilot-instructions.md) |
| `--gemini` | Include Gemini configuration (gemini-instructions.md) |
| `--amp` | Include AMP configuration (amp-context.md) |
| `--cline` | Include Cline configuration (.cline/rules.md) |
| `--continue-dev` | Include Continue.dev configuration (.continue/rules/) |
| `--codex` | Include Codex configuration (codex-context.md) |
| `--opencode` | Include OpenCode configuration (AGENTS.md) |
| `--use-agent <name>` | Use AI agent for intelligent configuration generation |
| `--no-agent` | Skip AI agent detection completely |
| `--list-agents` | List available AI agents and exit |
| `--with-agents` | Include agent configurations (creates specialized sub-agents) |
| `--with-sections` | Include documentation sections (enabled by default) |
| `--setup-hooks` | Configure git hooks for automatic validation |
| `--yes` | Skip all confirmation prompts (useful for scripting) |

#### AI Agent Support

When using `--use-agent`, the command will attempt to use your installed AI CLI tools:

| Agent | Command Used | Capabilities |
|-------|-------------|--------------|
| **claude** | `claude --print` | Comprehensive project analysis, generates detailed rules and sections |
| **gemini** | `gemini --prompt` | Fast configuration generation, good at detecting patterns |
| **amp** | `amp --execute` | Focused on development workflow optimization |
| **continue** | `cn --print` | Specialized for code context and development practices |

**Requirements:**
- The AI CLI tool must be installed and available in your PATH
- The tool must support non-interactive mode (print/prompt mode)
- Internet connection required for AI analysis

#### Example Outputs

**Basic init** creates a minimal configuration:
```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json
metadata:
  name: "My Project"
outputs:
  - path: "CLAUDE.md"
```

**AI-powered init** with `--use-agent claude --popular` creates:
```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json
metadata:
  name: "My Project"  
  description: "Go-based microservice with REST API and PostgreSQL backend"
presets:
  - "popular"
rules:
  - name: "Go Code Standards"
    priority: high
    content: "Follow standard Go project layout (cmd/, internal/, pkg/)..."
  - name: "API Design"  
    priority: critical
    content: "RESTful APIs should follow OpenAPI 3.0 specification..."
sections:
  - name: "Architecture Overview"
    priority: critical
    content: "This service implements clean architecture with..."
agents:
  - name: "go-expert"
    description: "Go language specialist for backend development"
    system_prompt: "You are a Go expert focusing on..."
```

#### Tips

- **Start simple**: Use `--popular` for most projects
- **Use AI assistance**: Add `--use-agent claude` for comprehensive, project-specific configuration
- **For automation**: Use `--yes` to skip prompts in CI/CD or scripts
- **Monorepos**: Run init in each service directory for focused configurations

### `generate`

Create all your AI instruction files and configure MCP servers.

```bash
ai-rulez generate

# Generate without updating .gitignore
ai-rulez generate --update-gitignore=false

# Generate files recursively in subdirectories
ai-rulez generate --recursive

# Preview what would be generated without creating files
ai-rulez generate --dry-run

# Skip CLI MCP tool configuration (only generate files)
ai-rulez generate --no-configure-cli-mcp
```

#### MCP Server Integration

By default, `generate` automatically configures MCP servers for available CLI tools:

```bash
ai-rulez generate
# ✅ Generated 3 file(s) successfully
# ✅ Configured claude MCP server: ai-rulez
# ✅ Configured gemini MCP server: database-tools
```

**Supported CLI tools:**
- **Claude CLI**: `claude mcp add` commands with environment variables
- **Gemini CLI**: `gemini mcp add` commands with automatic configuration

Use `--no-configure-cli-mcp` (or `--skip-cli-mcp` alias) to disable CLI configuration while still generating config files for other tools.

#### Options:
- `--dry-run` - Preview files that would be generated without creating them
- `--update-gitignore` - Control whether to update `.gitignore` (overrides config setting)
- `--recursive` - Process all `ai-rulez.yml` files in subdirectories
- `--no-configure-cli-mcp` - Skip configuring CLI-based MCP tools (Claude, Gemini, etc.)
- `--skip-cli-mcp` - Alias for `--no-configure-cli-mcp`

#### Gitignore Behavior:
By default, `generate` will automatically add all generated files to `.gitignore` to keep your repository clean. This includes:
- Files from presets (like `CLAUDE.md`, `.cursor/rules/`, etc.)
- Custom output files you've defined
- Agent directories and files

You can control this behavior:
1. **In your config**: Set `gitignore: false` to disable by default
2. **Via CLI**: Use `--update-gitignore=false` to disable for one run (overrides config)

When using `--recursive`, each directory's `.gitignore` is updated with its own generated files.

### `validate`

Check your configuration for errors.

```bash
ai-rulez validate
```

### `version`

Show the current version.

```bash
ai-rulez version
```

---

## Configuration Management (CRUD)

Manage every aspect of your `ai-rulez.yml` configuration directly from the command line.

### Rules

Manage the `rules` list.

```bash
# Add a new rule with an ID and targets
ai-rulez add rule "Code Style" --id "style-guide" --content "Follow the official Go style guide." --priority high --target "*.go"

# Get a specific rule
ai-rulez get rule "Code Style"

# List all rules
ai-rulez list rules

# Update a rule's content
ai-rulez update rule "Code Style" --content "Follow the official Go style guide, no exceptions."

# Delete a rule
ai-rulez delete rule "Code Style"
```

### Sections

Manage the `sections` list.

```bash
# Add a new section
ai-rulez add section "Project Overview" --id "proj-overview" --content "This project uses a clean architecture..."

# List all sections
ai-rulez list sections

# Delete a section
ai-rulez delete section "Project Overview"
```

### Agents

Manage the `agents` list.

```bash
# Add a new agent with a structured template
ai-rulez add agent "doc-writer" --description "Technical documentation writer" --template-type inline --template-value "Write clear documentation for the following code..."

# List all agents
ai-rulez list agents

# Delete an agent
ai-rulez delete agent "doc-writer"
```

### Outputs

Manage the `outputs` list.

```bash
# Add a directory output with a naming scheme and type
ai-rulez add output ".claude/agents/" --type agent --naming-scheme "{name}.md"

# List all outputs
ai-rulez list outputs

# Delete an output
ai-rulez delete output ".claude/agents/"
```

### MCP Servers

Manage the `mcp_servers` list.

```bash
# Add a new MCP server
ai-rulez add mcp-server "github" --command "npx" --arg "-y" --arg "@mcp/server-github" --env "TOKEN=123"

# List all MCP servers
ai-rulez list mcp-servers

# Delete an MCP server
ai-rulez delete mcp-server "github"
```

### Custom Commands

Manage the `commands` list.

```bash
# Add a new custom command with an alias and shortcut
ai-rulez add command "new-task" --description "Starts a new task" --alias "nt" --shortcut "Ctrl+Shift+N"

# List all custom commands
ai-rulez list commands

# Delete a custom command
ai-rulez delete command "new-task"
```

### Metadata

Manage the top-level `metadata` property.

```bash
# Get the current metadata
ai-rulez get metadata

# Set the project name and version
ai-rulez set metadata --name "My Awesome Project" --version "2.0.0"
```

### Extends

Manage the top-level `extends` property.

```bash
# Get the current extends path
ai-rulez get extends

# Set the extends path to a remote file
ai-rulez set extends "https://example.com/base-config.yaml"

# Remove the extends property
ai-rulez delete extends
```

### Includes

Manage the top-level `includes` list.

```bash
# List all included files
ai-rulez list includes

# Add a new file to the includes list
ai-rulez add include "./shared/common-rules.yaml"

# Remove a file from the includes list
ai-rulez delete include "./shared/common-rules.yaml"
```

---

## MCP Server

Starts the Model Context Protocol (MCP) server to allow AI assistants to programmatically interact with your configuration.

```bash
# Start the MCP server on the default port
ai-rulez mcp
```

See the [MCP Server Documentation](mcp-server.md) for more details.
