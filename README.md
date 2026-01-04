# ai-rulez

Write AI assistant rules once. Generate configs for 18 different tools.

Every AI coding tool wants its own config format. Claude needs `CLAUDE.md`, Cursor wants `.cursor/rules/`, Copilot expects `.github/copilot-instructions.md`. Keeping them in sync is tedious and error-prone.

ai-rulez solves this: define rules and context in `.ai-rulez/`, run `generate`, and get native configs for all your tools.

```bash
npx ai-rulez@latest init && npx ai-rulez@latest generate
```

**[Documentation](https://goldziher.github.io/ai-rulez/)**

## What You Get

- **18 preset generators**: Claude, Cursor, Windsurf, Copilot, Gemini, Cline, Continue.dev, Amp, Junie, Codex, OpenCode, and custom presets
- **Commands system**: Define slash commands once, use them across tools that support it
- **Context compression**: 34% size reduction with smart whitespace optimization
- **Remote includes**: Pull shared rules from git repos (company standards, team configs)
- **Profile system**: Generate different configs for backend/frontend/QA teams
- **MCP server**: Let AI assistants manage their own rules via Model Context Protocol
- **Type-safe schemas**: JSON Schema validation for all config files

## Quick Start

```bash
# No install required
npx ai-rulez@latest init "My Project"
npx ai-rulez@latest generate
```

This creates:

```
.ai-rulez/
├── config.yaml       # Which tools to generate for
├── rules/            # Guidelines AI must follow
├── context/          # Project background info
├── skills/           # Specialized AI roles
├── agents/           # Agent-specific prompts
└── commands/         # Slash commands
```

And generates native configs for each tool you specify.

## Configuration

```yaml
# .ai-rulez/config.yaml
version: "3.0"
name: "My Project"

presets:
  - claude
  - cursor
  - copilot
  - windsurf

# Optional: team-specific profiles
profiles:
  backend: [backend, database]
  frontend: [frontend, ui]

# Optional: share rules across repos
includes:
  - name: company-standards
    source: https://github.com/company/ai-rules.git
    ref: main
```

## Content Structure

**Rules** - What AI must do:
```markdown
---
priority: critical
---
# Security Standards
- Never commit credentials
- Use environment variables for secrets
- Sanitize all user input
```

**Context** - What AI should know:
```markdown
---
priority: high
---
# Architecture
This is a microservices app:
- API Gateway (Go, port 8080)
- Auth Service (Go, port 8081)
- PostgreSQL 15
```

**Commands** - Slash commands across tools:
```markdown
---
name: review
aliases: [r, pr-review]
targets: [claude, cursor, continue-dev]
---
# Code Review
Review the current PR for:
1. Logic errors
2. Security issues
3. Performance problems
```

## Installation

**No install required:**
```bash
npx ai-rulez@latest <command>
# or
uvx ai-rulez <command>
```

**Global install:**
```bash
# Homebrew
brew install goldziher/tap/ai-rulez

# npm
npm install -g ai-rulez

# pip
pip install ai-rulez

# Go
go install github.com/Goldziher/ai-rulez/cmd@latest
```

## CLI Reference

```bash
# Initialize project
ai-rulez init "Project Name"
ai-rulez init --domains backend,frontend,qa

# Generate configs
ai-rulez generate
ai-rulez generate --profile backend
ai-rulez generate --dry-run

# Content management
ai-rulez add rule security-standards --priority critical
ai-rulez add context api-docs
ai-rulez add skill database-expert
ai-rulez add command review-pr

ai-rulez list rules
ai-rulez remove rule outdated-rule

# Validation
ai-rulez validate

# MCP server (for AI assistants)
npx ai-rulez@latest mcp

# Migrate from V2
ai-rulez migrate v3
```

## Remote Includes

Share rules across repositories:

```yaml
includes:
  # HTTPS
  - name: company-standards
    source: https://github.com/company/ai-rules.git
    ref: main
    include: [rules, context]
    merge_strategy: local-override

  # SSH
  - name: shared-configs
    source: git@github.com:org/shared-ai-rulez.git
    ref: v2.0.0
    include: [rules, skills]

  # Local path
  - name: local-standards
    source: ../shared-rules
    include: [rules]
```

Private repos use `AI_RULEZ_GIT_TOKEN` environment variable or `--token` flag.

## Generated Output

Running `ai-rulez generate` creates:

| Preset | Output |
|--------|--------|
| Claude | `CLAUDE.md` + `.claude/skills/` + `.claude/agents/` |
| Cursor | `.cursor/rules/*.mdc` |
| Windsurf | `.windsurf/*.md` |
| Copilot | `.github/copilot-instructions.md` |
| Gemini | `.gemini/config.yaml` |
| Continue.dev | `.continue/prompts/ai_rulez_prompts.yaml` |
| Cline | `.cline/rules/*.md` |
| Custom | Any path with markdown, JSON, or directory output |

## Use Cases

**Monorepo**: Generate configs for multiple packages
```bash
ai-rulez generate --recursive
```

**Team profiles**: Different rules for different teams
```bash
ai-rulez generate --profile backend
ai-rulez generate --profile frontend
```

**CI validation**: Ensure configs stay in sync
```bash
ai-rulez validate && ai-rulez generate --dry-run
```

**Import existing configs**: Migrate from tool-specific files
```bash
ai-rulez init --from auto
ai-rulez init --from .cursorrules,CLAUDE.md
```

## MCP Server

Let AI assistants manage rules directly:

```yaml
# .ai-rulez/mcp.yaml
version: "3.0"
mcp_servers:
  - name: ai-rulez
    command: npx
    args: ["-y", "ai-rulez@latest", "mcp"]
    transport: stdio
    enabled: true
```

The MCP server exposes CRUD operations, validation, and generation to AI assistants.

## Compression

Reduce context size for token-constrained tools:

```yaml
compression:
  level: standard  # none, minimal, standard, aggressive
```

At `standard` level, output is ~34% smaller through whitespace optimization and duplicate removal.

## Documentation

- [Configuration Reference](https://goldziher.github.io/ai-rulez/configuration/)
- [Domains & Profiles](https://goldziher.github.io/ai-rulez/domains/)
- [Remote Includes](https://goldziher.github.io/ai-rulez/includes/)
- [MCP Server](https://goldziher.github.io/ai-rulez/mcp-server/)
- [Schema Validation](https://goldziher.github.io/ai-rulez/schema/)
- [Migration Guide](https://goldziher.github.io/ai-rulez/migration/)

## Contributing

Contributions welcome. See [CONTRIBUTING.md](https://github.com/Goldziher/ai-rulez/blob/main/CONTRIBUTING.md).

## License

MIT
