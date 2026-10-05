---
priority: high
summary: High-level overview of V4 config, profiles, presets, includes, and typical AI-Rulez workflows.
targets:
  - CLAUDE.md
  - GEMINI.md
  - .cursor/rules/*
  - .devin/*
  - .github/copilot-instructions.md
  - AGENTS.md
  - .hermes.md
---

# AI-Rulez Overview

AI-Rulez centralizes AI assistant governance in the `.ai-rulez/` directory and generates tool-specific outputs
for 52 built-in harness presets, including Claude Code, Cursor, Codex, Copilot, Gemini CLI, OpenCode, Devin, and Kilo
(`docs/harnesses.md` has the per-preset feature matrix).

Key concepts:

- V4 config lives in `.ai-rulez/config.toml` plus `rules/`, `context/`, `skills/`, `agents/`, `commands/`, and `checks/`.
- Profiles select which domains under `.ai-rulez/domains/` are included in a generation.
- Presets define output formats and paths; `ai-rulez generate` renders all configured presets. Most presets are declarative provider specs under `internal/generator/providers/builtin/*.toml`; ten are Go presets in `internal/generator/presets`.
- Includes let you merge shared rule sets into local content before generation.
- MCP server settings live inline in `.ai-rulez/config.toml` as `[[mcp_servers]]` entries and can be generated alongside presets.
- `[[hooks]]` and `[permissions]` are written once and translated into each harness's native format (36 and 24 harnesses). A rule or hook a harness cannot express is skipped with a warning, never approximated.
- `checks` are code-review guidelines rendered for the review tools that read a repository file.
- `ai-rulez generate --user` renders a user config into per-user directories; settings files you edit by hand are merged key by key and keep their comments.
- Removed or renamed presets: `windsurf` is `devin`; `continue-dev` is gone.

Typical workflow:

1. Update `.ai-rulez/` sources (config, rules, context, skills, agents, checks, domains).
2. Run `ai-rulez generate` (optionally `--profile`, `--dry-run` or `--watch`).
3. Commit `.ai-rulez/` and generated outputs together.

Migration:

- V2 used a single `ai-rulez.yaml`; use `ai-rulez migrate v4` to convert to V4 directory-based configuration when needed.
