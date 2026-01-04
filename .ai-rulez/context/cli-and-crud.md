---
priority: high
summary: Core commands (init, generate, validate, migrate, mcp) and CRUD helpers for managing configuration.
targets:
  - CLAUDE.md
  - GEMINI.md
  - .cursor/rules/*
  - .windsurf/*
---

# CLI and CRUD Commands

Core commands:
- `ai-rulez init` initializes `.ai-rulez/` with optional presets or domains.
- `ai-rulez generate` renders outputs; `--profile` selects domains; `--dry-run` previews.
- `ai-rulez validate` checks config and content structure.
- `ai-rulez migrate` converts V2 `ai-rulez.yaml` to V3 `.ai-rulez/`.
- `ai-rulez mcp` starts the MCP server (usually launched by the assistant).

CRUD helpers manage file-based content:
- `ai-rulez add rule|context|skill|agent` creates content files.
- `ai-rulez remove rule|context|skill|agent` removes content files.
- `ai-rulez list rules|context|skills|agents` lists items.
- `ai-rulez domain add|remove|list` and `ai-rulez profile add|remove|list` manage scopes.
- `ai-rulez include add|remove|list` manages remote includes.

Global flags:
- `--config` overrides config discovery.
- `--debug`, `--verbose`, and `--quiet` control logging and progress.

Config discovery checks for `.ai-rulez/` first, then V2 filenames (`ai-rulez.yaml`, etc).
