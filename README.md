<p align="center">
  <img src="docs/assets/logo.png" alt="AI-Rulez" width="200" />
</p>

<h1 align="center">ai-rulez</h1>

<p align="center">
  <strong>Directory-based AI governance for 19+ tools</strong>
</p>

<p align="center">
  <a href="https://goreportcard.com/report/github.com/Goldziher/ai-rulez"><img src="https://goreportcard.com/badge/github.com/Goldziher/ai-rulez" alt="Go Report Card"></a>
  <a href="https://www.npmjs.com/package/ai-rulez"><img src="https://img.shields.io/npm/v/ai-rulez" alt="npm version"></a>
  <a href="https://pypi.org/project/ai-rulez/"><img src="https://img.shields.io/pypi/v/ai-rulez" alt="PyPI version"></a>
  <a href="https://github.com/Goldziher/ai-rulez/blob/main/LICENSE"><img src="https://img.shields.io/github/license/Goldziher/ai-rulez" alt="License"></a>
  <a href="https://goldziher.github.io/ai-rulez/"><img src="https://img.shields.io/badge/docs-ai--rulez-blue" alt="Documentation"></a>
</p>

<p align="center">
  Define rules, context, skills, agents and commands once — generate native configs for Claude, Cursor, Copilot, Windsurf, Gemini, Codex, and more.
</p>

---

Every AI coding tool wants its own config format. Claude needs `CLAUDE.md` + `.claude/skills/` + `.claude/agents/`, Cursor wants `.cursor/rules/`, Copilot expects `.github/copilot-instructions.md`. Keeping them in sync is tedious and error-prone.

**ai-rulez** solves this: organize your AI governance in `.ai-rulez/`, run `generate`, and get native configs for all your tools — with proper frontmatter, tool-specific formatting, and full feature support (skills, agents, MCP servers, plugins).

```bash
npx ai-rulez@latest init && npx ai-rulez@latest generate
```

<p align="center">
  <a href="https://goldziher.github.io/ai-rulez/"><strong>Documentation</strong></a> &middot;
  <a href="https://goldziher.github.io/ai-rulez/quick-start/"><strong>Quick Start</strong></a> &middot;
  <a href="https://goldziher.github.io/ai-rulez/examples/"><strong>Examples</strong></a>
</p>

## What You Get

- **19 preset generators**: Claude, Cursor, Windsurf, Copilot, Gemini, Antigravity, Cline, Continue.dev, Amp, Junie, Codex, OpenCode, and custom presets
- **Commands system**: Define slash commands once, use them across tools that support it
- **Concise builtins**: Optimized builtin rules for minimal token footprint
- **Remote includes**: Pull shared rules from git repos (company standards, team configs)
- **Profile system**: Generate different configs for backend/frontend/QA teams
- **MCP server**: Let AI assistants manage their own rules via Model Context Protocol
- **Plugins & Marketplaces**: Extend ai-rulez with custom generators and install pre-built rulesets from marketplaces
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
├── config.toml       # Which tools to generate for
├── rules/            # Guidelines AI must follow
├── context/          # Project background info
├── skills/           # Specialized AI roles
├── agents/           # Agent-specific prompts
└── commands/         # Slash commands
```

And generates native configs for each tool you specify.

## Configuration

```toml
# .ai-rulez/config.toml
version = "4.0"
name = "My Project"

presets = ["claude", "cursor", "copilot", "windsurf"]

builtins = ["security", "testing"]

# Optional: team-specific profiles
[profiles]
backend = ["backend", "database"]
frontend = ["frontend", "ui"]

# Optional: share rules across repos
[[includes]]
name = "company-standards"
source = "https://github.com/company/ai-rules.git"
ref = "main"

# Optional: inline MCP servers
[[mcp_servers]]
name = "ai-rulez"
command = "npx"
args = ["-y", "ai-rulez@latest", "mcp"]
transport = "stdio"
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

# Installed skills
ai-rulez skill install kreuzberg --source https://github.com/kreuzberg-dev/kreuzberg
ai-rulez skill list
ai-rulez skill remove kreuzberg

# Validation
ai-rulez validate

# MCP server (for AI assistants)
npx ai-rulez@latest mcp

```

## Remote Includes

Share rules across repositories:

```toml
# HTTPS
[[includes]]
name = "company-standards"
source = "https://github.com/company/ai-rules.git"
ref = "main"
include = ["rules", "context"]
merge_strategy = "local-override"

# SSH
[[includes]]
name = "shared-configs"
source = "git@github.com:org/shared-ai-rulez.git"
ref = "v2.0.0"
include = ["rules", "skills"]

# Local path
[[includes]]
name = "local-standards"
source = "../shared-rules"
include = ["rules"]
```

Private repos use `AI_RULEZ_GIT_TOKEN` environment variable or `--token` flag.

## Installed Skills

Install named skills from external repositories — fetched dynamically at generate time:

```toml
[[installed_skills]]
name = "kreuzberg"
source = "https://github.com/kreuzberg-dev/kreuzberg"

[[installed_skills]]
name = "ai-rulez"
source = "https://github.com/Goldziher/ai-rulez"
```

```bash
ai-rulez skill install kreuzberg --source https://github.com/kreuzberg-dev/kreuzberg
ai-rulez skill list
ai-rulez skill remove kreuzberg
```

Skills live at `skills/<name>/SKILL.md` in the source repo. See [Installed Skills docs](https://goldziher.github.io/ai-rulez/installed-skills/) for details.

## Generated Output

Running `ai-rulez generate` creates:

| Preset | Output |
|--------|--------|
| Claude | `CLAUDE.md` + `.claude/skills/` + `.claude/agents/` |
| Cursor | `.cursor/rules/*.mdc` |
| Windsurf | `.windsurf/*.md` |
| Copilot | `.github/copilot-instructions.md` |
| Gemini | `GEMINI.md` |
| Continue.dev | `.continue/prompts/ai_rulez_prompts.yaml` |
| Cline | `.cline/rules/*.md` |
| Codex | `AGENTS.md` |
| Amp | `AMP.md` |
| Junie | `.junie/guidelines.md` |
| OpenCode | `OPENCODE.md` |
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

Let AI assistants manage rules directly. MCP servers are configured inline in `config.toml`:

```toml
[[mcp_servers]]
name = "ai-rulez"
command = "npx"
args = ["-y", "ai-rulez@latest", "mcp"]
transport = "stdio"
enabled = true

[[mcp_servers]]
name = "custom-server"
command = "python"
args = ["-m", "my_mcp_server"]
transport = "stdio"
```

The MCP server exposes CRUD operations, validation, and generation to AI assistants.

## Builtins

27 built-in domains ship embedded in the binary — opinionated conventions ready to use without external includes:

```toml
builtins = ["rust", "python", "typescript", "security", "testing", "default-commands"]
```

- **Universal** (9): `ai-governance`\*, `agent-delegation`\*, `security`, `git-workflow`, `code-quality`, `testing`, `token-efficiency`, `documentation`, `default-commands`
- **Languages** (10): `rust`, `python`, `typescript`, `go`, `java`, `ruby`, `php`, `elixir`, `csharp`, `r`
- **Bindings** (9): `pyo3`, `napi-rs`, `magnus`, `ext-php-rs`, `rustler`, `wasm`, `jni-rs`, `extendr`, `cgo`

\* Auto-included by default. Use `builtins: true` for all, or pick specific ones. Exclude auto-includes with `!` prefix (e.g., `!agent-delegation`).

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
