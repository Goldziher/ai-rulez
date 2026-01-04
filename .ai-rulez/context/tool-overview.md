---
priority: high
summary: High-level overview of V3 config, profiles, presets, includes, and typical AI-Rulez workflows.
targets:
  - CLAUDE.md
  - GEMINI.md
  - .cursor/rules/*
  - .windsurf/*
  - .github/copilot-instructions.md
---

# AI-Rulez Overview

AI-Rulez centralizes AI assistant governance in the `.ai-rulez/` directory and generates tool-specific outputs
for Claude, Cursor, Windsurf, Copilot, Gemini, and other presets.

Key concepts:
- V3 config lives in `.ai-rulez/config.yaml` plus `rules/`, `context/`, `skills/`, and `agents/`.
- Profiles select which domains under `.ai-rulez/domains/` are included in a generation.
- Presets define output formats and paths; `ai-rulez generate` renders all configured presets.
- Includes let you merge shared rule sets into local content before generation.
- MCP server settings live in `.ai-rulez/mcp.yaml` and can be generated alongside presets.

Typical workflow:
1. Update `.ai-rulez/` sources (config, rules, context, skills, agents, domains).
2. Run `ai-rulez generate` (optionally `--profile` or `--dry-run`).
3. Commit `.ai-rulez/` and generated outputs together.

Migration:
- V2 used a single `ai-rulez.yaml`; use `ai-rulez migrate v3` to convert to V3 directories when needed.
