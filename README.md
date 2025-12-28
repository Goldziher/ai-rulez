# ai-rulez

Directory-based AI governance for development teams.

ai-rulez lets you write AI assistant rules, context, and instructions once, then generate native configuration files for Claude, Cursor, Windsurf, Copilot, Gemini, and other AI coding tools. All configuration lives in a single `.ai-rulez/` directory in your repository.

**Documentation:** https://goldziher.github.io/ai-rulez/

## Quick Start

```bash
# Initialize (no installation required)
npx ai-rulez@latest init
# or
uvx ai-rulez init

# Edit your configuration in .ai-rulez/
# Then generate tool-specific configs
npx ai-rulez@latest generate
# or
uvx ai-rulez generate
```

<details>
<summary>Installation Methods</summary>

### Global Installation

**Homebrew (macOS/Linux):**
```bash
brew install goldziher/tap/ai-rulez
```

**Go:**
```bash
go install github.com/Goldziher/ai-rulez/cmd@latest
```

**npm:**
```bash
npm install -g ai-rulez
```

**Python (pip):**
```bash
pip install ai-rulez
```

**Python (uv):**
```bash
uv tool install ai-rulez
```

</details>

## How It Works

### 1. Initialize Your Project

```bash
ai-rulez init "My Project"
```

This creates a `.ai-rulez/` directory:

```
.ai-rulez/
├── config.yaml           # Main configuration
├── rules/                # Rules that AI assistants must follow
│   └── code-quality.md
├── context/              # Background information about your project
│   └── architecture.md
├── skills/               # Specialized AI agent configurations
│   └── code-reviewer/
│       └── SKILL.md
├── agents/               # Agent prompt files
├── domains/              # Optional domain-specific organization
└── mcp.yaml              # Model Context Protocol servers
```

<details>
<summary>Init Command Options</summary>

```bash
# Create with specific domains
ai-rulez init --domains backend,frontend,qa

# Skip example content
ai-rulez init --skip-content

# Use JSON instead of YAML
ai-rulez init --format json

# Import from existing tool files
ai-rulez init --from auto
ai-rulez init --from .cursorrules,CLAUDE.md
```

</details>

### 2. Configure Your AI Tools

Edit `.ai-rulez/config.yaml` to specify which AI tools you want to support:

```yaml
version: "3.0"
name: "My Project"

presets:
  - claude
  - cursor
  - copilot
```

<details>
<summary>Available Presets</summary>

- **claude** - Claude Desktop and Claude Code
- **cursor** - Cursor IDE
- **windsurf** - Windsurf IDE
- **copilot** - GitHub Copilot
- **gemini** - Google Gemini
- **cline** - Cline extension
- **continue-dev** - Continue.dev
- **junie** - Junie AI
- **amp** - AMP
- **codex** - OpenAI Codex
- **opencode** - OpenCode

</details>

### 3. Add Your Rules and Context

Create markdown files in `.ai-rulez/rules/`, `.ai-rulez/context/`, `.ai-rulez/skills/`, and `.ai-rulez/agents/`:

```markdown
---
priority: high
---

# Code Quality Standards

- Use descriptive variable names
- Add comments for complex logic
- Write unit tests for new functions
```

<details>
<summary>Content Types Explained</summary>

**Rules** (`.ai-rulez/rules/`)

Strict guidelines that AI assistants must follow. These are prescriptive and enforce behavior.

```markdown
---
priority: critical
---

# Security Standards

Never commit credentials or API keys to the repository.
Use environment variables for all secrets.
```

**Context** (`.ai-rulez/context/`)

Background information about your project that helps AI understand your codebase.

```markdown
---
priority: medium
---

# Project Architecture

This project uses microservices architecture:
- API Gateway (port 8080)
- Auth Service (port 8081)
- Database: PostgreSQL 15
```

**Skills** (`.ai-rulez/skills/`)

Specialized AI agent configurations for specific tasks. Each skill creates a focused agent with access to all project rules and context.

```markdown
---
name: database-expert
description: PostgreSQL optimization specialist
priority: high
---

# Database Expert

You are an expert in PostgreSQL optimization and query design.
Focus on performance, indexing strategies, and query optimization.
```

**Agents** (`.ai-rulez/agents/`)

Agent prompt files used by tools that support agent-specific instructions.

```markdown
---
priority: high
---

# Code Reviewer Agent

Review pull requests with a focus on correctness and maintainability.
```

</details>

### 4. Generate Tool Configurations

```bash
ai-rulez generate
```

This creates native configuration files for each preset you specified.

<details>
<summary>What Gets Generated - Detailed Output Structure</summary>

### Claude Preset

Generates a structured directory with skills and agents:

```
.claude/
├── skills/
│   └── {skill-id}/
│       └── SKILL.md      # Skill with embedded rules and context
└── agents/
    └── {skill-id}.md     # Agent file (backward compatibility)
```

Each skill file contains:
- Skill-specific instructions
- All project rules
- All project context

### Cursor Preset

Generates individual rule files:

```
.cursor/
└── rules/
    ├── code-quality.mdc
    ├── security.mdc
    └── architecture.mdc
```

Each rule is a separate `.mdc` file with priority and content.

### Windsurf Preset

Generates individual rule files:

```
.windsurf/
├── code-quality.md
├── security.md
└── architecture.md
```

Each rule is a separate markdown file.

### Copilot Preset

Generates a single consolidated file:

```
.github/
└── copilot-instructions.md
```

Contains all rules, context, and skills in one file organized by sections.

### Gemini Preset

Generates configuration in Gemini's expected format:

```
.gemini/
└── config.yaml
```

### MCP Preset (Auto-generated)

When you configure MCP servers in `.ai-rulez/mcp.yaml`, an MCP configuration is automatically generated for tools that support Model Context Protocol.

</details>

## Features

### Multi-Tool Support
Generate native configuration files for all major AI coding assistants from a single source of truth.

### Domain-Based Organization
Separate rules and context by domain (backend, frontend, QA, etc.) to keep large codebases organized.

<details>
<summary>Using Domains</summary>

Create domain-specific organization:

```bash
# Create domains during init
ai-rulez init --domains backend,frontend,qa

# Or add domains later
ai-rulez add domain backend
```

Directory structure:

```
.ai-rulez/domains/
├── backend/
│   ├── rules/
│   ├── context/
│   └── skills/
└── frontend/
    ├── rules/
    ├── context/
    └── skills/
```

Configure in `config.yaml`:

```yaml
profiles:
  backend: [backend, database]
  frontend: [frontend, ui]
  full: [backend, frontend, database, ui]

default: full
```

Generate for specific domain:

```bash
ai-rulez generate --profile backend
```

</details>

### Profiles
Define multiple profiles for different teams or use cases. Generate only the rules relevant to each context.

<details>
<summary>Profile Configuration</summary>

```yaml
version: "3.0"
name: "My Project"

presets:
  - claude
  - cursor

profiles:
  backend: [backend, database]
  frontend: [frontend, ui]
  full: [backend, frontend, database, ui]

default: full
```

Generate specific profile:

```bash
ai-rulez generate --profile backend
```

</details>

### Priority System
Mark rules and context with priority levels (critical, high, medium, low) to control their importance and ordering.

### Includes
Compose configurations from multiple sources including local paths, Git repositories, and shared rule packages.

<details>
<summary>Using Includes</summary>

Reference external rule sources:

```yaml
includes:
  - name: company-standards
    source: https://github.com/company/ai-rules.git
    ref: main

  - name: local-standards
    source: ../shared-rules
```

See the [Includes documentation](https://goldziher.github.io/ai-rulez/includes/) for more details.

</details>

### MCP Server Integration
Configure Model Context Protocol servers that extend AI assistant capabilities with external tools and data sources.

<details>
<summary>MCP Configuration</summary>

Configure MCP servers in `.ai-rulez/mcp.yaml`:

```yaml
version: "3.0"

mcp_servers:
  - name: ai-rulez
    description: AI-Rulez MCP server for configuration management
    command: npx
    args:
      - "-y"
      - "ai-rulez@latest"
      - "mcp"
    transport: stdio
    enabled: true

  - name: github
    description: GitHub integration
    command: npx
    args:
      - "-y"
      - "@modelcontextprotocol/server-github"
    env:
      GITHUB_PERSONAL_ACCESS_TOKEN: "${GITHUB_TOKEN}"
    transport: stdio
    enabled: true
```

MCP servers can also be domain-specific. See the [MCP documentation](https://goldziher.github.io/ai-rulez/mcp-server/) for details.

</details>

### Import from Existing Tools
Migrate from existing tool-specific configurations.

<details>
<summary>Import Options</summary>

```bash
# Auto-detect existing configs
ai-rulez init --from auto

# Import specific files
ai-rulez init --from .cursorrules,CLAUDE.md

# Import from multiple sources
ai-rulez init --from .cursorrules,.windsurfrules
```

</details>

### Monorepo Support
Generate configurations recursively for multiple projects.

<details>
<summary>Recursive Generation</summary>

```bash
ai-rulez generate --recursive
```

This finds all `.ai-rulez/` directories in your repository and generates configurations for each.

</details>

### Git Hook Integration
Automatically validate your AI rules configuration on commit.

<details>
<summary>Git Hooks Setup</summary>

```bash
ai-rulez init --setup-hooks
```

Supports:
- lefthook
- pre-commit
- husky

Automatically detects your git hook manager and configures validation.

</details>

### CLI Management
Full CRUD operations for managing your configuration programmatically.

<details>
<summary>CLI Commands Reference</summary>

**Add Content:**
```bash
ai-rulez add rule security-standards --priority critical
ai-rulez add context api-endpoints
ai-rulez add skill test-engineer
ai-rulez add domain backend
```

**List Content:**
```bash
ai-rulez list rules
ai-rulez list context
ai-rulez list skills
ai-rulez list domains
```

**Remove Content:**
```bash
ai-rulez remove rule outdated-rule
ai-rulez remove domain legacy
```

**Validation:**
```bash
ai-rulez validate
```

**MCP Server:**
```bash
npx ai-rulez@latest mcp
# or
uvx ai-rulez mcp
```

Configure in your AI assistant's MCP settings to enable direct management of your AI rules.

</details>

### Migration from V2
Migrate from older ai-rulez V2 configurations to the new V3 directory-based structure.

<details>
<summary>Migration Guide</summary>

```bash
# Auto-detect V2 config and migrate
ai-rulez migrate v3

# Specify config file
ai-rulez migrate v3 ai-rulez.yaml

# Preview migration without writing files
ai-rulez migrate v3 --dry-run

# Force migration if .ai-rulez exists (creates backup)
ai-rulez migrate v3 --force
```

The migrate command:
- Converts your V2 `ai-rulez.yaml` to the new `.ai-rulez/` directory structure
- Preserves all rules, context, and configuration
- Creates timestamped backups when using `--force`
- Validates the migration automatically

</details>

## Example Workflows

<details>
<summary>Start a New Project</summary>

```bash
# Initialize with domains
ai-rulez init "My API" --domains backend,frontend,qa

# Edit .ai-rulez/config.yaml and add your presets
# Add project-specific rules
ai-rulez add rule api-standards --priority high

# Generate configs for all tools
ai-rulez generate
```

</details>

<details>
<summary>Migrate from Existing Configs</summary>

```bash
# Import your existing configs
ai-rulez init --from auto

# Review the imported content
ls .ai-rulez/rules/

# Add more presets to config.yaml
# Then regenerate
ai-rulez generate
```

</details>

<details>
<summary>Team-Specific Profiles</summary>

```bash
# Set up profiles in .ai-rulez/config.yaml
# profiles:
#   backend: [backend, database]
#   frontend: [frontend, ui]

# Backend team generates their config
ai-rulez generate --profile backend

# Frontend team generates their config
ai-rulez generate --profile frontend
```

</details>

<details>
<summary>Monorepo with Multiple Projects</summary>

```bash
# Initialize in each project subdirectory
cd packages/api
ai-rulez init "API Service" --domains backend

cd ../web
ai-rulez init "Web App" --domains frontend

# Generate all at once from root
cd ../..
ai-rulez generate --recursive
```

</details>

## Documentation

Full documentation is available at https://goldziher.github.io/ai-rulez/

Key topics:
- [Configuration Reference](https://goldziher.github.io/ai-rulez/configuration/)
- [Domain Organization](https://goldziher.github.io/ai-rulez/domains/)
- [Profile Management](https://goldziher.github.io/ai-rulez/profiles/)
- [MCP Server Setup](https://goldziher.github.io/ai-rulez/mcp-server/)
- [Includes and Composition](https://goldziher.github.io/ai-rulez/includes/)
- [Monorepo Usage](https://goldziher.github.io/ai-rulez/monorepo/)
- [Schema Validation](https://goldziher.github.io/ai-rulez/schema/)

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](https://github.com/Goldziher/ai-rulez/blob/main/CONTRIBUTING.md) for development setup and guidelines.

## License

MIT
