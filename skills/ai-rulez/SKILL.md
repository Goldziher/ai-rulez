---
name: ai-rulez
description: >-
  Manage AI assistant governance rules across Claude, Cursor, Windsurf,
  Copilot, Gemini, and other tools using ai-rulez. Use when configuring
  rules, context, skills, domains, profiles, includes, or generating
  tool-specific outputs.
license: MIT
metadata:
  author: Goldziher
  version: "3.11"
  repository: https://github.com/Goldziher/ai-rulez
---

# AI-Rulez Governance

AI-Rulez centralizes AI assistant governance in a `.ai-rulez/` directory and generates tool-specific outputs for Claude, Cursor, Windsurf, Copilot, Gemini, and other presets.

Use this skill when:

- Setting up or modifying `.ai-rulez/` configuration
- Writing rules, context, skills, or agents for AI assistants
- Configuring domains, profiles, or includes
- Generating outputs for specific AI tools
- Installing or managing external skills

## Installation

```bash
# Go (primary CLI)
go install github.com/Goldziher/ai-rulez@latest

# npm
npx ai-rulez@latest

# Python
uvx ai-rulez
```

## Quick Start

```bash
# Initialize a new project
ai-rulez init

# Add rules and context
ai-rulez add rule my-rule --priority high
ai-rulez add context my-context

# Generate outputs for all configured presets
ai-rulez generate

# Validate configuration
ai-rulez validate
```

## Configuration Structure

```
.ai-rulez/
  config.yaml          # Main configuration
  mcp.yaml             # MCP server definitions (optional)
  rules/               # Governance rules (.md files with frontmatter)
  context/             # Contextual information (.md files)
  skills/              # Specialized capabilities (name/SKILL.md)
  agents/              # Agent definitions (.md files)
  commands/            # Custom commands (.md files)
  domains/             # Domain-scoped content
    backend/
      rules/
      context/
      skills/
```

## config.yaml

```yaml
version: "3.0"
name: my-project
description: Project description
presets:
  - claude
  - cursor
  - gemini
profiles:
  backend: [backend, shared]
  frontend: [frontend, shared]
default: backend
builtins: [go, security, testing]
installed_skills:
  - name: kreuzberg
    source: https://github.com/kreuzberg-dev/kreuzberg
includes:
  - name: shared-rules
    source: https://github.com/org/shared-rules
```

## Content Frontmatter

Rules, context, skills, and agents support YAML frontmatter:

```yaml
---
priority: high          # critical, high, medium, low
targets:                # Limit to specific presets
  - CLAUDE.md
  - .cursor/rules/*
description: Brief description
---
```

## Domains and Profiles

Domains group content for different teams or concerns. Profiles select which domains are active.

```bash
ai-rulez domain add backend
ai-rulez add rule api-standards --domain backend
ai-rulez profile add backend-team backend,shared
ai-rulez generate --profile backend-team
```

## Includes

Share rules across projects using git repositories or local paths:

```bash
ai-rulez include add shared-rules https://github.com/org/shared-rules
ai-rulez include add local-rules ./path/to/local --merge-strategy include-override
```

## Installed Skills

Install named skills from external repositories:

```bash
ai-rulez skill install kreuzberg --source https://github.com/kreuzberg-dev/kreuzberg
ai-rulez skill install ai-rulez --source https://github.com/Goldziher/ai-rulez
ai-rulez skill list
ai-rulez skill remove kreuzberg
```

Skills are fetched dynamically at generation time and included in outputs.

## Built-in Presets

Available presets: `claude`, `cursor`, `gemini`, `copilot`, `continue-dev`, `windsurf`, `cline`, `codex`, `amp`, `junie`, `opencode`.

## MCP Server

ai-rulez exposes an MCP server for AI assistants to read, create, update, and generate configuration:

```bash
ai-rulez mcp
```

Configure in your AI tool's MCP settings to enable CRUD operations from within the assistant.
