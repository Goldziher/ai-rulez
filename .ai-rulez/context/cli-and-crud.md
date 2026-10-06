---
priority: high
summary: Core commands (init, generate, validate, migrate, mcp) and CRUD helpers for managing configuration.
targets:
  - CLAUDE.md
  - GEMINI.md
  - .cursor/rules/*
  - .devin/*
  - AGENTS.md
  - .hermes.md
---

# CLI and CRUD Commands

Core commands:

- `ai-rulez init` initializes `.ai-rulez/` with optional presets or domains.
- `ai-rulez generate` renders outputs; `--profile` selects domains; `--dry-run` previews; `--watch` regenerates on change; `--check` reports drift without writing; `--user` renders the user config into the home directory.
- `ai-rulez clean` removes generated outputs (the inverse of `generate`); `--dry-run` previews, `--force` skips the prompt; `--user` removes what `generate --user` wrote.
- `ai-rulez validate` checks config and content structure (`--strict` adds deep content checks).
- `ai-rulez doctor` runs read-only diagnostics (removed presets, drift, unresolved MCP placeholders, missing tools); it exits 2 on errors.
- `ai-rulez verify` checks generated files against their `Content-Hash` offline.
- `ai-rulez migrate v4` converts a V3 `.ai-rulez/config.yaml` to V4 TOML config (a flat `ai-rulez.yaml` is not read; move it to `.ai-rulez/config.yaml` first). `-C` selects another project's config.
- `ai-rulez mcp` starts the MCP server (usually launched by the assistant).

CRUD helpers manage file-based content:

- `ai-rulez add rule|context|skill|agent|command|check` creates content files.
- `ai-rulez remove rule|context|skill|agent|command|check` removes content files.
- `ai-rulez list rules|context|skills|agents|commands|checks` lists items.
- `ai-rulez domain add|remove|list` and `ai-rulez profile add|remove|list` manage scopes.
- `ai-rulez include add|remove|list` manages remote includes.

Global flags:

- `--config` overrides config discovery.
- `--debug`, `--verbose`, and `--quiet` control logging and progress.

Config discovery checks for `.ai-rulez/` first, then V2 filenames (`ai-rulez.yaml`, etc).
