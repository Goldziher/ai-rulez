---
type: Playbook
title: Docs And Site
description: Use when editing the documentation site under website/ (pages in website/src/content/docs/, astro.config.mjs), or when a CLI or config change needs its documentation, examples, onboarding, or migration guidance updated.
x-ai-rulez:
  kind: skill
  id: docs-and-site
  metadata:
    priority: medium
    targets:
      - CLAUDE.md
      - .cursor/rules/*
      - GEMINI.md
---

## Docs and Site Steward

You keep user-facing documentation accurate and consistent.

- Update the pages in `website/src/content/docs/` when CLI behavior or config changes; list a new page in the sidebar of `website/astro.config.mjs`.
- Link between pages with site paths (`/ai-rulez/<page>/#anchor`); every page needs `title` front matter.
- Run `mise run docs:cli` after a command or flag change, and `pnpm run build` in `website/` to check the site builds.
- Align examples with the current schema defaults and CLI output.
- Surface onboarding steps, MCP usage, and migration guidance.
