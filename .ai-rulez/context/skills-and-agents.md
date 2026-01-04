---
priority: medium
summary: Specialized skills for focused task guidance and tool-specific agent definitions.
targets:
  - CLAUDE.md
  - .cursor/rules/*
  - GEMINI.md
---

# Skills and Agents

- Skills live in `.ai-rulez/skills/{name}/SKILL.md` and describe specialized roles or workflows.
- Agents live in `.ai-rulez/agents/*.md` and map to tool-specific agent definitions when supported.
- Both are included in generated outputs alongside rules and context.

Use skills for focused task guidance; use agents when the target tool supports multi-agent prompts.
