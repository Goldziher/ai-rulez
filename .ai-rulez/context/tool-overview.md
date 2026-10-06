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
- `ai-rulez.lock` pins remote includes, installed skills, skill sources, authored content and generated outputs; it is enforced whenever it exists. `includes`, installed skills and skill sources accept `version = "^1.2"` ranges that `ai-rulez update` moves.
- `[[roles]]` select a slice of the content (`generate --role`); `delivery = "served"` skills are served on demand by `ai-rulez mcp --serve-skills` instead of written to the harness trees.
- `validate --strict` and `scan` run deep content and security checks with stable `AR` codes; `[[verifiers]]` run deterministic repo checks; `sbom`, `catalog`, `tokens` and `cost` report what the configuration contains and costs.
- v5 behaviour: the Go module is `github.com/Goldziher/ai-rulez/v5`; `http://` and `git://` remotes are rejected and the git token goes only to allowlisted hosts; a committed config cannot point outside the project; content symlinks must resolve inside the project; Claude MCP servers are written only to `.mcp.json`; exit codes are `0` ok, `1` could not run, `2` findings or drift, `3` unpinned served skills (`lock`).
- Removed or renamed presets: `windsurf` is `devin`; `continue-dev` is gone.

Typical workflow:

1. Update `.ai-rulez/` sources (config, rules, context, skills, agents, checks, domains).
2. Run `ai-rulez generate` (optionally `--profile`, `--dry-run` or `--watch`).
3. Commit `.ai-rulez/` and generated outputs together.

Migration:

- V2 used a single `ai-rulez.yaml`; move it to `.ai-rulez/config.yaml`, then run `ai-rulez migrate v4` to convert to V4 directory-based configuration.
