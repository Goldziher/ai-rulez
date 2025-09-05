# CLI Reference

All the `ai-rulez` commands you need to know.

## Core Commands

The essentials for day-to-day use.

### `init`

Create a new `ai-rulez.yml` file in the current directory with an intelligent template.

```bash
ai-rulez init "My Project" --popular
```

The `init` command generates a comprehensive template where only required fields (metadata and outputs) are uncommented. All optional features are provided as well-documented, commented examples that you can selectively enable.

#### How it works:
1. **Analyzes your codebase** - Detects tech stack, build tools, and project structure
2. **Generates smart template** - Creates a rich YAML template with inline documentation
3. **Optional AI assistance** - If you have Claude, Gemini, or AMP installed, it can intelligently customize the template based on your project

#### Options:
- `--popular` - Enable popular AI tools (Claude, Cursor, Windsurf, Copilot)
- `--preset <name>` - Use a specific preset (claude, cursor, gemini, amp, etc.)
- `--use-agent <agent>` - Use an AI agent to customize the template (claude, continue, gemini, amp)
- `--yes` - Skip confirmation prompts

#### AI Agent Support:
Currently supports:
- **Claude** - Uses `claude --print` command
- **Continue.dev** - Uses `cn --print` command
- **Gemini** - Uses `gemini --prompt` command  
- **AMP** - Uses `amp --execute` command

Note: Codex and Cursor agents are not supported for template generation as they require interactive mode or special authentication that doesn't work well with automated generation.

### `generate`

Create all your AI instruction files.

```bash
ai-rulez generate

# Generate without updating .gitignore
ai-rulez generate --update-gitignore=false

# Generate files recursively in subdirectories
ai-rulez generate --recursive

# Preview what would be generated without creating files
ai-rulez generate --dry-run
```

#### Options:
- `--dry-run` - Preview files that would be generated without creating them
- `--update-gitignore` - Control whether to update `.gitignore` (overrides config setting)
- `--recursive` - Process all `ai-rulez.yml` files in subdirectories

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
