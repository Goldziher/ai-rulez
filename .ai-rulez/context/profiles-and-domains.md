---
priority: high
summary: Domain organization, profile configuration, and team-based output tailoring.
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
---

# Profiles and Domains

- Root content under `.ai-rulez/rules`, `context`, `skills`, `agents`, and `commands` is always included.
- Domain content lives under `.ai-rulez/domains/{name}/` and is included when the domain is in the active profile. Builtin domains and domains sourced from external includes (`FromInclude`) are always included regardless of the active profile.
- Profiles are defined in `.ai-rulez/config.toml`.
- The built-in `default` profile behaves as follows:
  - If the user explicitly defines `profiles: { "default": [...] }` in config, that definition is honoured like any named profile.
  - If **no profiles** are defined, `default` includes **root content and all domains** (including domains from includes).
  - If profiles **are** defined but `"default"` is not among them, `default` includes **root content, builtin domains, and domains sourced from external includes (`FromInclude`)**.
- MCP server definitions are configured at the project level with `[[mcp_servers]]`.

Use profiles to tailor outputs for different teams (e.g., `backend`, `frontend`, `qa`).
