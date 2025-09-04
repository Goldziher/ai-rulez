# CLI Reference

All the `ai-rulez` commands you need to know.

## Core Commands

The essentials for day-to-day use.

### `init`

Create a new `ai_rulez.yaml` file in the current directory.

```bash
ai-rulez init "My Project" --popular
```

### `generate`

Create all your AI instruction files.

```bash
ai-rulez generate
```

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

Manage every aspect of your `ai_rulez.yaml` configuration directly from the command line.

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
