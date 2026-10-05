---
priority: high
summary: Config loading, profile resolution, preset generation, and output rendering workflow.
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
  - AGENTS.md
  - .hermes.md
---

# Generation Pipeline

- `internal/config` loads `.ai-rulez/config.toml`, scans content trees, and resolves includes.
- `internal/generator` selects profiles, collects MCP servers, and renders presets.
- Preset generators live under `internal/generator/presets` and use templates from `internal/templates`. Declarative provider specs (`internal/generator/providers/builtin/*.toml`) define most of the 52 presets.
- Settings files (`.claude/settings.json`, `opencode.json`, `.codex/config.toml`, ...) are merged document by document: ai-rulez owns the keys, array elements and map members it writes and preserves the rest, comments included (JSONC, TOML, YAML).
- Two presets writing the same path must render identical bytes; `generate` fails and names them otherwise.
- Output writing updates `.gitignore` when `gitignore: true` is set in config.

Rendering flow:

1. Load config and content tree.
2. Resolve profile (default or specified).
3. Generate preset outputs (plus MCP output when servers exist).
4. Write files and update gitignore.

Profile notes:

- `default` profile resolution order:
  1. If `profiles["default"]` is explicitly defined in config → use it like any named profile.
  2. If no profiles are defined at all → include root content and all domains (including included domains).
  3. If other profiles exist but `"default"` is not defined → include root content, builtin domains, and `FromInclude` domains.
- `ai-rulez generate --profile <name>` builds root content plus the named profile's domains, along with builtin and `FromInclude` domains.
