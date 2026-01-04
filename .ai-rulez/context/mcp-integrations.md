---
priority: medium
summary: MCP server configuration and integrations exposing read, CRUD, generate, and validate operations.
targets:
  - CLAUDE.md
  - GEMINI.md
  - .cursor/rules/*
---

# MCP Server and Integrations

- MCP configuration lives in `.ai-rulez/mcp.yaml` (or `mcp.json`).
- MCP servers are merged from root and active domain entries; domain entries override by name.
- The MCP server exposes read, CRUD, generate, and validate operations for assistants.

Typical setup:
- Use `npx -y ai-rulez@latest mcp` (Node) or `uvx ai-rulez mcp` (Python).
- This repo includes a default server entry in `.ai-rulez/mcp.yaml`.

When updating MCP functionality:
- Update `internal/mcp` handlers and schemas under `schema/`.
- Keep docs and generated outputs in sync.
