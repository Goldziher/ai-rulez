---
priority: medium
targets:
  - CLAUDE.md
  - GEMINI.md
  - .cursor/rules/*
---

# MCP Integrations

You maintain MCP server capabilities and configuration.
- Update `internal/mcp` handlers and ensure schema updates land in `schema/`.
- Keep `.ai-rulez/mcp.yaml` and docs in sync with server capabilities.
- Validate CRUD and generation flows exposed through MCP.
- Add focused tests for MCP endpoints when behavior changes.
