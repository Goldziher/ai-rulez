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

## MCP Server and Integrations

- MCP server configuration lives inline in `.ai-rulez/config.toml` as `[[mcp_servers]]` entries.
- Separate `.ai-rulez/mcp.yaml`, `mcp.toml` and `mcp.json` files are no longer read; declare servers as `[[mcp_servers]]`.
- MCP server definitions are project-level config entries. Domain content does not currently define or override MCP servers.
- The MCP server exposes read, CRUD, generate, and validate operations for assistants. With `source = "agents"` and `agents_md_split = true`, the context tools address one `##` section of the root `AGENTS.md` as an item.
- Remote servers (an HTTP `url`) can be published as MCP Server Cards with `publish --emit mcp-server-card` (`internal/mcpcard`, `docs/mcp-server-card.md`); stdio servers are skipped and header and environment values are never written.

Typical setup:

- Use `ai-rulez mcp` from the reviewed native CLI. CI pins the reviewed source revision; packages use the documented distribution channels.
- This repo includes a default server entry in `.ai-rulez/config.toml`.

When updating MCP functionality:

- Update `internal/mcp` handlers and schemas under `schema/`.
- Keep docs and generated outputs in sync.
