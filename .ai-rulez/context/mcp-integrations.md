---
priority: medium
summary: MCP server configuration and integrations exposing read, CRUD, generate, and validate operations.
targets:
  - CLAUDE.md
  - GEMINI.md
  - .cursor/rules/*
---

# MCP Server and Integrations

- MCP server configuration lives inline in `.ai-rulez/config.toml` as `[[mcp_servers]]` entries.
- Legacy `.ai-rulez/mcp.yaml`, `.ai-rulez/mcp.toml`, and `.ai-rulez/mcp.json` files are loaded for backward compatibility during migration and config loading.
- MCP server definitions are project-level config entries. Domain content does not currently define or override MCP servers.
- The MCP server exposes read, CRUD, generate, and validate operations for assistants.

Typical setup:

- Use `npx -y ai-rulez@latest mcp` (Node) or `uvx ai-rulez mcp` (Python).
- This repo includes a default server entry in `.ai-rulez/config.toml`.

When updating MCP functionality:

- Update `internal/mcp` handlers and schemas under `schema/`.
- Keep docs and generated outputs in sync.
