---
type: Concept
title: Mcp Integrations
x-ai-rulez:
  kind: context
  id: mcp-integrations
  metadata:
    priority: medium
    targets:
      - CLAUDE.md
      - GEMINI.md
      - .cursor/rules/*
      - AGENTS.md
      - .hermes.md
    summary: MCP server configuration and integrations exposing read, CRUD, generate, and validate operations.
---

# MCP Server and Integrations

- MCP server configuration lives inline in `.ai-rulez/config.toml` as `[[mcp_servers]]` entries.
- Separate `.ai-rulez/mcp.yaml`, `mcp.toml` and `mcp.json` files are no longer read; declare servers as `[[mcp_servers]]`.
- MCP server definitions are project-level config entries. Domain content does not currently define or override MCP servers.
- The MCP server exposes read, CRUD, generate, and validate operations for assistants.

Typical setup:

- Use `npx -y ai-rulez@latest mcp` (Node) or `uvx ai-rulez mcp` (Python).
- This repo includes a default server entry in `.ai-rulez/config.toml`.

When updating MCP functionality:

- Update `internal/mcp` handlers and schemas under `schema/`.
- Keep docs and generated outputs in sync.
