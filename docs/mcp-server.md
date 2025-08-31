# MCP Server

ai-rulez includes a built-in MCP server for dynamic configuration management with AI assistants.

## Quick Start

```bash
ai-rulez mcp
```

## Integration

### Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "ai-rulez",
      "args": ["mcp"]
    }
  }
}
```

### Claude Code

```bash
claude mcp add ai-rulez "ai-rulez" "mcp"
```

### Via Package Managers

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "npx",
      "args": ["-y", "ai-rulez@latest", "mcp"]
    }
  }
}
```

## Available Tools

The MCP server provides 19 tools for managing configuration:

### Configuration Retrieval
- **`get_rules`** - List all rules (supports `min_priority`, `name_filter` parameters)
- **`get_sections`** - List all sections (supports `name` filter parameter)
- **`get_agents`** - List all agents (supports `name` filter parameter)
- **`get_outputs`** - List all outputs (supports `path` filter parameter)

### Add Elements
- **`add_rule`** - Add new rule (`content` required, `section`, `priority` optional)
- **`add_section`** - Add new section (`name` required, `priority` optional)
- **`add_agent`** - Add new agent (`name`, `description` required, `priority`, `tools`, `system_prompt` optional)
- **`add_output`** - Add new output (`path` required, `type`, `naming_scheme` optional)

### Update Elements (Index-Based)
- **`update_rule`** - Update rule by index (`index`, `content` required, `priority` optional)
- **`update_section`** - Update section by index (`index`, `name` required, `priority` optional)
- **`update_agent`** - Update agent by index (`index` required, all other fields optional)
- **`update_output`** - Update output by index (`index` required, all other fields optional)

### Delete Elements (Index-Based)
- **`delete_rule`** - Delete rule by index (`index` required)
- **`delete_section`** - Delete section by index (`index` required)
- **`delete_agent`** - Delete agent by index (`index` required)
- **`delete_output`** - Delete output by index (`index` required)

### Project Operations
- **`generate_output`** - Generate files (`config_file` optional)
- **`validate_config`** - Validate configuration (`config_file` required)
- **`init_project`** - Initialize project (`name` required, `preset`, `use_agent` optional)
- **`get_version`** - Get ai-rulez version (no parameters)

## Usage Examples

Simply ask your AI assistant to manage configurations:

> "Add a rule with content 'Write unit tests' and priority 8"

> "Update rule at index 0 with new content 'Write comprehensive unit tests'"

> "Generate the AI files from my configuration"

> "Get all rules with priority higher than 5"

> "Initialize a new project called 'My API' with preset claude"

**Note**: Update and delete operations use zero-based indices. Use `get_*` tools first to find the correct index.

## Benefits

- **Dynamic Updates** - No file editing required
- **Real-time Generation** - Immediate file generation after changes  
- **Automatic Validation** - All changes are validated
- **Seamless Integration** - Natural AI assistant workflow

## Custom Configuration

```bash
ai-rulez mcp --config custom.yaml
```