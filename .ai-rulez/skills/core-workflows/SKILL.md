---
priority: high
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
---

# Core Workflows

1. Configuration loading reads `.ai-rulez/config.yaml` via `internal/config`, scans content trees, and resolves includes.
2. Generation uses `internal/generator` and `internal/generator/presets` with `internal/templates` to render outputs and `internal/gitignore` to update ignore rules.
3. CRUD subcommands in `cmd/commands` call `internal/crud` to create or update rules, context, skills, agents, domains, profiles, and includes.
4. Validation runs through `internal/validator` and the config loaders to check schema and structure.
5. MCP handlers in `internal/mcp` expose read, CRUD, generate, and validate operations to external tools.
