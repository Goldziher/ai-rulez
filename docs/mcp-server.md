# Enabling the MCP Server

The `ai-rulez` MCP (Model Context Protocol) server allows your AI assistant to programmatically and safely interact with your `.ai-rulez/` configuration. Since V3 uses a file-based approach, you edit configuration files directly with your editor, and the MCP server provides read-only access for AI assistants to generate outputs and validate your configuration.

**You do not need to start the server manually.** Your AI assistant will start it automatically based on the configuration you provide.

---

## Configuration Examples

To enable the server, add one of the following snippets to your AI assistant's configuration file (e.g., Cursor's `settings.json`).

### Using `npx` (Recommended for Node.js users)

This method ensures you are always using the latest version of `ai-rulez` without needing to install it globally.

```json
{
  "mcp_servers": {
    "ai-rulez": {
      "command": "npx",
      "args": ["-y", "ai-rulez@latest", "mcp"]
    }
  }
}
```

### Using `uvx` (Recommended for Python users)

This method uses `uvx` to run `ai-rulez` in an ephemeral environment.

```json
{
  "mcp_servers": {
    "ai-rulez": {
      "command": "uvx",
      "args": ["ai-rulez", "mcp"]
    }
  }
}
```

### Using a Local Go Installation

If you have installed `ai-rulez` locally with `go install`.

```json
{
  "mcp_servers": {
    "ai-rulez": {
      "command": "ai-rulez",
      "args": ["mcp"]
    }
  }
}
```

---

## Server Capabilities

When enabled, the MCP server provides your AI assistant with read-only access to your configuration. The server supports:

- **Read Configuration**: Inspect rules, context, skills, profiles, and presets
- **Generate Outputs**: Programmatically trigger generation of tool-specific files
- **Validate Configuration**: Check configuration validity and report errors

Since V3 uses a file-based approach (`.ai-rulez/` directory), configuration changes are made by directly editing files. The MCP server enables AI assistants to:

1. **Understand your setup** by reading configuration and content files
2. **Generate outputs** after you've made changes
3. **Validate changes** before committing

This approach ensures your configuration remains auditable and version-controlled, while still allowing AI assistants to help you manage it efficiently.

## Typical Workflow

### With Your Editor

1. **Edit files** directly in `.ai-rulez/`:
   ```bash
   # Edit rules, context, skills, or config.yaml
   vim .ai-rulez/rules/code-quality.md
   ```

2. **Use MCP server** (via AI assistant) to generate:
   ```bash
   ai-rulez generate
   ```

3. **Commit changes**:
   ```bash
   git add .ai-rulez/ CLAUDE.md .cursor/
   git commit -m "docs: update AI guidelines"
   ```

### With Claude CLI

If using the Claude CLI with the ai-rulez MCP server, you can ask Claude to help:

- "Review my `.ai-rulez/` configuration and suggest improvements"
- "Generate outputs for my new rules"
- "Validate that my profiles are correct"

The server provides Claude with read access to understand your setup, while you maintain full control over edits.
