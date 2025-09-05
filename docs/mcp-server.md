# Enabling the MCP Server

The `ai-rulez` MCP (Model Context Protocol) server allows your AI assistant to programmatically and safely interact with your `ai-rulez.yml` configuration. Instead of asking the AI to manually edit the YAML file, the assistant can use the server to add rules, update agents, or generate files directly.

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

When enabled, the MCP server provides your AI assistant with a comprehensive set of tools to safely manage your entire `ai-rulez.yml` configuration. The assistant can:

- **Manage Rules, Sections, and Agents:** Add, update, delete, and list all core configuration elements.
- **Manage Outputs:** Add or remove new output targets for generation.
- **Manage Tools:** Configure MCP servers and custom slash commands.
- **Manage Metadata:** Get or set top-level properties like the project name, `extends`, and `includes`.
- **Trigger Core Actions:** Programmatically run the `generate` and `validate` commands.

This allows for powerful, automated workflows, such as asking your AI assistant to "add a new rule for our Python standards and then regenerate the output files."
