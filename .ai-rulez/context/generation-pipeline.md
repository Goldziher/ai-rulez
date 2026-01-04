---
priority: high
summary: Config loading, profile resolution, preset generation, and output rendering workflow.
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
---

# Generation Pipeline (V3)

- `internal/config` loads `.ai-rulez/config.yaml`, scans content trees, and resolves includes.
- `internal/generator` selects profiles, collects MCP servers, and renders presets.
- Preset generators live under `internal/generator/presets` and use templates from `internal/templates`.
- Output writing updates `.gitignore` when `gitignore: true` is set in config.

Rendering flow:
1. Load config and content tree.
2. Resolve profile (default or specified).
3. Generate preset outputs (plus MCP output when servers exist).
4. Write files and update gitignore.

Profile notes:
- `default` profile means root content only (no domains).
- `ai-rulez generate --profile <name>` builds the root content plus the profile's domains.
