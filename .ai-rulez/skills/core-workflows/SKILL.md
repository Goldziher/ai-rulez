---
description: "Core Workflows"
priority: high
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
---

# Core Workflows

1. Configuration loading reads `.ai-rulez/config.toml` via `internal/config`, scans content trees, and resolves includes.
2. Generation uses declarative specs in `internal/generator/providers/builtin/` plus specialized generators in `internal/generator/presets`, with `internal/templates` rendering outputs and `internal/gitignore` updating ignore rules.
3. CRUD subcommands in `cmd/commands` call `internal/crud` to create or update rules, context, skills, agents, domains, profiles, and includes.
4. Validation runs through `internal/validator` and the config loaders to check schema and structure.
5. MCP handlers in `internal/mcp` expose read, CRUD, generate, and validate operations to external tools.
