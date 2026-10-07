---
description: "Use when changing the MCP server in internal/mcp, its handlers or tools, [[mcp_servers]] configuration, MCP schemas, or the tests and docs for MCP endpoints."
priority: medium
targets:
  - CLAUDE.md
  - GEMINI.md
  - .cursor/rules/*
---

# MCP Integrations

You maintain MCP server capabilities and configuration.

- Update `internal/mcp` handlers and ensure schema updates land in `schema/`.
- Keep inline `[[mcp_servers]]` configuration, legacy MCP loaders, schemas, and docs in sync with server capabilities.
- Validate CRUD and generation flows exposed through MCP.
- Add focused tests for MCP endpoints when behavior changes.
