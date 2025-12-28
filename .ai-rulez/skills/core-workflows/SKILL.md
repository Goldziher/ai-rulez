---
priority: high
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
---

# Core Workflows

1. Configuration generation loads `ai-rulez.yaml` through `internal/config`, renders templates from `internal/templates`, and writes outputs via `internal/generator`.
2. Enforcement creates an `enforcement.Enforcer`, analyzes files with the selected agent, and reports results through the reporter pipeline.
3. CRUD subcommands manipulate YAML using the config loader/saver with detailed diagnostics.
4. MCP handlers in `internal/mcp` expose generation and enforcement to external tools.
