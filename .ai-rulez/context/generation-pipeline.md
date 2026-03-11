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

- `default` profile resolution order:
  1. If `profiles["default"]` is explicitly defined in config → use it like any named profile.
  2. If no profiles are defined at all → include root content and all domains (including included domains).
  3. If other profiles exist but `"default"` is not defined → include root content, builtin domains, and `FromInclude` domains.
- `ai-rulez generate --profile <name>` builds root content plus the named profile's domains, along with builtin and `FromInclude` domains.
