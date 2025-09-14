# ai-rulez ⚡

<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/logo.png" alt="ai-rulez logo" width="200" style="border-radius: 15%; overflow: hidden;">
</p>

**One config to rule them all.**

## The Problem

If you're using multiple AI coding assistants (Claude Code, Cursor, Windsurf, GitHub Copilot, OpenCode), you've probably noticed the configuration fragmentation. Each tool demands its own format - `CLAUDE.md`, `.cursorrules`, `.windsurfrules`, `.github/copilot-instructions.md`, `AGENTS.md`. Keeping coding standards consistent across all these tools is frustrating and error-prone.

## The Solution

AI-Rulez lets you write your project configuration once and automatically generates native files for every AI tool - current and future ones. It's like having a build system for AI context.

<p align="center">
  <img src="docs/assets/ai-rulez-python-demo.gif" alt="AI-Rulez Demo" width="100%">
</p>

## Why This Matters

Development teams using AI assistants face common challenges:
- **Multiple tools, multiple configs**: Your team uses Claude Code for reviews, Cursor for development, Copilot for completions
- **Configuration drift**: Maintaining separate files leads to inconsistent standards across tools
- **Monorepo complexity**: Multiple services and packages all need different AI contexts
- **Team consistency**: Junior devs get different AI guidance than seniors
- **Future-proofing**: New AI tools require rewriting all configurations

AI-Rulez solves this with a single `ai-rulez.yaml` that understands your project's conventions.

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8)](https://go.dev)
[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-orange)](https://github.com/Goldziher/homebrew-tap)

### 📖 **[Read the Full Documentation](https://goldziher.github.io/ai-rulez/)**

---

## Key Features

### AI-Powered Project Analysis
The `init` command is where AI-Rulez shines. Instead of manually writing configurations, let AI analyze your codebase:

```bash
# AI analyzes your codebase and generates tailored config
npx ai-rulez init "My Project" --preset popular --use-agent claude --yes
```

This automatically:
- Detects your tech stack (Python/Node/Go, testing frameworks, linters)
- Identifies project patterns and conventions
- Generates appropriate coding standards and practices
- Creates specialized agents for different tasks (code review, testing, docs)
- **Automatically adds all generated AI files to .gitignore** - no more committing `.cursorrules` or `CLAUDE.md` by accident

### Universal Output Generation
One YAML config generates files for every tool:
- `CLAUDE.md` for Claude Code
- `.cursorrules` for Cursor
- `.windsurfrules` for Windsurf  
- `.github/copilot-instructions.md` for GitHub Copilot
- Custom formats for any future AI tool

### Powerful Enterprise Features
- **MCP Integration:** Automatically configure MCP servers across CLI tools (Claude, Gemini) and generate config files for others (Cursor, VS Code). One configuration, every tool connected.
- **Team Collaboration:** Remote config includes, local overrides, and monorepo support with `--recursive`
- **Full-Featured CLI:** Manage your entire configuration from the command line. Add rules, update agents, and generate files without ever opening a YAML file.
- **Security & Performance:** SSRF protection, schema validation, Go-based performance with instant startup

## How It Works

`ai-rulez` takes your `ai-rulez.yml` file and uses it as a single source of truth to generate native configuration files for all your AI tools. Think of it as a build system for AI context—you write the source once, and it compiles to whatever format each tool needs.

## Example: `ai-rulez.yml`

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "My SaaS Platform"
  version: "2.0.0"

# Use presets for common configurations
presets:
  - "popular"  # Includes Claude, Cursor, Windsurf, Copilot, and Gemini

rules:
  - name: "Go Code Standards"
    priority: high
    content: "Follow standard Go project layout (cmd/, internal/, pkg/). Use meaningful package names and export only what is necessary."

sections:
  - name: "Project Structure"
    priority: critical
    content: |
      - `cmd/`: Main application entry point
      - `internal/`: Private application code (business logic, data access)
      - `pkg/`: Public-facing libraries

agents:
  - name: "go-developer"
    description: "Go language expert for core development"
    system_prompt: "You are an expert Go developer. Your key responsibilities include writing idiomatic Go, using proper error handling, and creating comprehensive tests."

# MCP servers for direct AI tool integration
mcp_servers:
  - name: "ai-rulez"
    command: "ai-rulez"
    args: ["mcp"]
    description: "AI-Rulez MCP server for configuration management"
```

Run `ai-rulez generate` → get all your configuration files, perfectly synchronized.

## Quick Start

```bash
# 1. AI-powered initialization (recommended)
ai-rulez init "My Project" --preset popular --use-agent claude

# 2. Generate all AI instruction files
ai-rulez generate

# 3. Your AI tools now have comprehensive, project-specific context!
```

**That's it!** The AI will analyze your codebase and generate tailored rules, documentation, and specialized agents automatically.

**Prefer manual setup?**
```bash
# Basic initialization without AI assistance
ai-rulez init "My Project" --preset popular

# Add your project-specific context  
ai-rulez add rule "Tech Stack" --priority critical --content "This project uses Go and PostgreSQL."

# Generate files
ai-rulez generate
```

## MCP Server Integration

`ai-rulez` provides seamless **Model Context Protocol (MCP)** integration, automatically configuring both file-based and CLI-based AI tools with your MCP servers.

### Automatic CLI Configuration

When you run `ai-rulez generate`, MCP servers are **automatically configured** for available CLI tools:

```bash
ai-rulez generate
# ✅ Generated 3 file(s) successfully
# ✅ Configured claude MCP server: ai-rulez
# ✅ Configured gemini MCP server: database-tools
```

**Supported CLI tools:**
- **Claude CLI**: `claude mcp add` with full env/transport support
- **Gemini CLI**: `gemini mcp add` with automatic configuration

### Hybrid Configuration

`ai-rulez` supports both CLI and file-based configurations simultaneously:

```yaml
mcp_servers:
  - name: "database-tools"
    command: "uvx"
    args: ["mcp-server-postgres"]
    env:
      DATABASE_URL: "postgresql://localhost/mydb"
    targets: 
      - "@claude-cli"        # Configure Claude CLI
      - "@gemini-cli"        # Configure Gemini CLI  
      - ".cursor/mcp.json"   # Generate Cursor config file
```

This single configuration:
- ✅ Executes `claude mcp add` commands
- ✅ Executes `gemini mcp add` commands  
- ✅ Generates `.cursor/mcp.json` file

### Control Options

**Default behavior** (recommended):
```bash
ai-rulez generate
# Configures all available CLI tools + generates files
```

**Disable CLI configuration** when needed:
```bash
ai-rulez generate --no-configure-cli-mcp
# Only generates files, skips CLI tool configuration
```

**Target specific tools:**
```yaml
mcp_servers:
  - name: "github-integration"
    command: "npx"
    args: ["@modelcontextprotocol/server-github"]
    targets: ["@claude-cli"]  # Only configure Claude CLI
```

### Built-in MCP Server

`ai-rulez` includes its own MCP server for configuration management:

```bash
# Start the ai-rulez MCP server
ai-rulez mcp

# Or configure it automatically via your ai-rulez.yaml
mcp_servers:
  - name: "ai-rulez"
    command: "ai-rulez" 
    args: ["mcp"]
    description: "Configuration management server"
```

## AI-Powered Rule Enforcement

AI-Rulez provides **real-time rule enforcement** using AI agents to automatically detect violations and apply fixes across your codebase.

### Basic Enforcement

```bash
# Check for violations (read-only by default)
ai-rulez enforce

# Automatically apply fixes
ai-rulez enforce --fix

# Use specific AI agent
ai-rulez enforce --agent gemini --fix
```

### Advanced Enforcement Options

```bash
# Enforce with specific level
ai-rulez enforce --level strict --agent claude

# Review workflow with iterative improvement
ai-rulez enforce --review --review-iterations 3 --review-threshold 85

# Multi-agent review (different agents for enforcement vs review)
ai-rulez enforce --agent gemini --review --review-agent claude

# Target specific files and rules
ai-rulez enforce --include-files "src/**/*.js" --only-rules "no-console-output"

# Output formats for automation
ai-rulez enforce --format json --output violations.json
ai-rulez enforce --format csv --output report.csv
```

### Supported AI Agents

AI-Rulez integrates with all major AI coding assistants:

- **Claude** (`claude`) - Anthropic's AI assistant
- **Gemini** (`gemini`) - Google's AI model
- **Cursor** (`cursor`) - AI-powered code editor
- **AMP** (`amp`) - Sourcegraph's AI assistant
- **Codex** (`codex`) - OpenAI's code model
- **Continue.dev** (`continue-dev`) - Open-source coding assistant

### Enforcement Levels

- **`warn`**: Log violations but don't fail (default)
- **`error`**: Fail on violations but don't auto-fix
- **`fix`**: Automatically apply suggested fixes
- **`strict`**: Fail immediately on any violation

### Integration with Git Hooks

Add enforcement to your Git workflow:

```yaml
# .lefthook.yml
pre-commit:
  commands:
    ai-rulez-enforce:
      run: ai-rulez enforce --level error --agent gemini
      stage_fixed: true
```

```bash
# Or with pre-commit hooks
# .pre-commit-config.yaml
repos:
  - repo: local
    hooks:
      - id: ai-rulez-enforce
        name: AI-Rulez Enforcement
        entry: ai-rulez enforce --level error
        language: system
        pass_filenames: false
```

### Review Workflow

The review system provides iterative code improvement:

```bash
# Enable review with quality scoring
ai-rulez enforce --review --review-threshold 80

# Multiple review iterations
ai-rulez enforce --review --review-iterations 5

# Auto-approve after reaching threshold
ai-rulez enforce --review --review-auto-approve

# Require improvement between iterations
ai-rulez enforce --review --require-improvement
```

The AI reviewer analyzes:
- ✅ Code quality and adherence to rules
- ✅ Suggested fixes and their appropriateness
- ✅ Overall improvement between iterations
- ✅ Compliance with project standards

## Installation

### Run without installing

For one-off executions, you can run `ai-rulez` directly without a system-wide installation.

**Go**
```bash
go run github.com/Goldziher/ai-rulez/cmd@latest --help
```

**Node.js (via npx)**
```bash
# Installs and runs the latest version
npx ai-rulez@latest init
```

**Python (via uvx)**
```bash
# Runs ai-rulez in a temporary virtual environment
uvx ai-rulez init
```

### Install globally

For frequent use, a global installation is recommended.

**Go**
```bash
go install github.com/Goldziher/ai-rulez/cmd@latest
```

**Homebrew (macOS/Linux)**
```bash
brew install goldziher/tap/ai-rulez
```

**npm**
```bash
npm install -g ai-rulez
```

**pip**
```bash
pip install ai-rulez
```

## Pre-commit Hooks

You can use `ai-rulez` with `pre-commit` to automatically validate and generate your AI configuration files.

Add the following to your `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v2.2.1
    hooks:
      - id: ai-rulez-validate
      - id: ai-rulez-generate
```

---

## Documentation

- **[Quick Start Guide](https://goldziher.github.io/ai-rulez/quick-start/)**
- **[Full CLI Reference](https://goldziher.github.io/ai-rulez/cli/)**
- **[Configuration Guide](https://goldziher.github.io/ai-rulez/configuration/)**
- **[Migration Guide](https://goldziher.github.io/ai-rulez/migration-guide/)** - Upgrading from v1.x to v2.0

## Contributing

Contributions are welcome! Please see the [Contributing Guide](CONTRIBUTING.md) to get started.
