---
type: Concept
title: Profiles And Domains
x-ai-rulez:
  kind: context
  id: profiles-and-domains
  metadata:
    priority: high
    targets:
      - CLAUDE.md
      - .cursor/rules/*
      - GEMINI.md
      - AGENTS.md
      - .hermes.md
    summary: Domain organization, profile configuration, and team-based output tailoring.
---

# Profiles and Domains

- Root content under `.ai-rulez/rules`, `context`, `skills`, `agents`, and `commands` is always included.
- Domain content lives under `.ai-rulez/domains/{name}/` and is included when the domain is in the active profile. Globally-active builtin domains and domains sourced from external includes (`FromInclude`) are always included regardless of the active profile.
- Profiles are defined in `.ai-rulez/config.toml`.
- A profile may reference a builtin pack as `builtin:<name>` (e.g. `builtin:rust`) to load it for that profile only, rather than globally. This works even when the root `builtins` field is absent or `false`, and the `builtin:` prefix avoids colliding with a local domain of the same name. Root `builtins` entries remain global.
- The built-in `default` profile behaves as follows:
  - If the user explicitly defines `profiles: { "default": [...] }` in config, that definition is honoured like any named profile.
  - If **no profiles** are defined, `default` includes **root content and all domains** (including domains from includes).
  - If profiles **are** defined but `"default"` is not among them, `default` includes **root content, globally-active builtin domains, and domains sourced from external includes (`FromInclude`)**. A profile-scoped builtin is excluded unless `default` names it.
- MCP server definitions are configured at the project level with `[[mcp_servers]]`.

Use profiles to tailor outputs for different teams (e.g., `backend`, `frontend`, `qa`).
