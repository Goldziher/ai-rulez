---
type: Concept
title: Skills And Agents
x-ai-rulez:
  kind: context
  id: skills-and-agents
  metadata:
    priority: medium
    targets:
      - CLAUDE.md
      - .cursor/rules/*
      - GEMINI.md
      - AGENTS.md
      - .hermes.md
    summary: Specialized skills for focused task guidance and tool-specific agent definitions.
---

## Skills and Agents

- Skills live in `.ai-rulez/skills/{name}/SKILL.md` and describe specialized roles or workflows.
- Agents live in `.ai-rulez/agents/*.md` and map to tool-specific agent definitions when supported.
- Both are included in generated outputs alongside rules and context.
- With `source = "agents"` the same kinds are read from the open standards instead: skills from `.agents/skills/<name>/SKILL.md` (Agent Skills), agents from `.agents/agents/`, rules from `.agents/rules/`, and context from the root and nested `AGENTS.md` and `.agents/context/`. `generate` never writes into `.agents/` or `AGENTS.md` for this source (`docs/agents-source.md`).
- Checks live in `.ai-rulez/checks/{name}.md` (frontmatter `description`, `severity`, `tools`, `targets`) and are rendered only for the review tools that read a repository file (`cursor`, `kilo`, `qwen`, `factory`, `rovodev`, `amp`, `augment`, `gitlab-duo`). Their outputs are committed, not gitignored.

Use skills for focused task guidance; use agents when the target tool supports multi-agent prompts.
