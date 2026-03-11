---
priority: high
summary: Domain organization, profile configuration, and team-based output tailoring.
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
---

# Profiles and Domains

- Root content under `.ai-rulez/rules`, `context`, `skills`, and `agents` is always included.
- Domain content lives under `.ai-rulez/domains/{name}/` and is included only when the domain is in the active profile.
- Profiles are defined in `.ai-rulez/config.yaml`.
- The built-in `default` profile behaves as follows:
  - If **no profiles** are defined, `default` includes **root content and all domains**.
  - If profiles **are** defined but no explicit `default` is set, `default` includes **root content and builtin domains only**.
- Domain MCP servers can override root MCP entries by name.

Use profiles to tailor outputs for different teams (e.g., `backend`, `frontend`, `qa`).
