# ai-rulez ⚡

<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/logo.png" alt="ai-rulez logo" width="200" style="border-radius: 15%; overflow: hidden;">
</p>

**AI-powered development governance. One config to rule them all.**

## The Complete AI Development Platform

AI-Rulez is the **definitive platform for AI-powered development governance**. Beyond just generating configuration files, it provides real-time rule enforcement, automated code quality assurance, and intelligent governance across your entire development workflow.

### 🚀 Two Powerful Capabilities

**1. Universal Configuration Management** - Write once, deploy everywhere
**2. AI-Powered Rule Enforcement** - Real-time governance with automated fixes

## Why AI-Rulez?

Modern development teams need more than just configuration management:

### ⚠️ **The Problem**
- **Configuration Hell**: Each AI tool needs its own format (`.cursorrules`, `CLAUDE.md`, `.windsurfrules`, etc.)
- **Rule Enforcement Gap**: No way to automatically validate that code follows your standards
- **Team Inconsistency**: Different developers get different AI guidance
- **Manual Quality Control**: Time-consuming code reviews for basic rule violations
- **Reactive Governance**: Finding issues after they're already committed

### ✨ **The Solution**
AI-Rulez provides **proactive development governance** with a single `ai-rulez.yaml` that:
- **Generates configurations** for every AI tool automatically
- **Enforces rules in real-time** using AI agents (Claude, Gemini, etc.)
- **Applies automatic fixes** for code quality issues
- **Integrates with your workflow** (Git hooks, CI/CD, pre-commit)
- **Scales across teams** with consistent standards

<p align="center">
  <img src="docs/assets/ai-rulez-python-demo.gif" alt="AI-Rulez Configuration Demo" width="100%">
  <br><em>📝 Configuration Management (via npx ai-rulez)</em>
</p>

<p align="center">
  <img src="docs/assets/ai-rulez-enforce-demo.gif" alt="AI-Rulez Enforcement Demo" width="100%">
  <br><em>🤖 AI-Powered Rule Enforcement (via uvx ai-rulez@latest)</em>
</p>

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8)](https://go.dev)
[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-orange)](https://github.com/Goldziher/homebrew-tap)

### 📖 **[Read the Full Documentation](https://goldziher.github.io/ai-rulez/)**

---

## 🚀 Core Capabilities

### 1. 📋 Universal Configuration Management

Write once, deploy everywhere with intelligent AI-powered project analysis:

```bash
# AI analyzes your codebase and generates tailored config
npx ai-rulez init "My Project" --preset popular
```

**Features:**
- **AI Project Analysis**: Automatically detects your tech stack, patterns, and conventions
- **Universal Output Generation**: One YAML → all AI tool formats (`CLAUDE.md`, `.cursorrules`, `.windsurfrules`, etc.)
- **Smart Gitignore Management**: Automatically excludes generated files from version control
- **MCP Integration**: Auto-configure MCP servers across CLI tools (Claude, Gemini, etc.)
- **Team Collaboration**: Remote includes, local overrides, monorepo support

### 2. 🤖 AI-Powered Rule Enforcement

Real-time governance with automated fixes using multiple AI agents:

```bash
# Check for violations with AI analysis
uvx ai-rulez@latest enforce --agent claude

# Automatically apply fixes
uvx ai-rulez@latest enforce --agent claude --fix

# Multi-agent review workflow
uvx ai-rulez@latest enforce --agent gemini --review --review-agent claude
```

**Features:**
- **Multi-Agent Support**: Claude, Gemini, AMP, Cursor, Codex, Continue.dev, Junie
- **Automated Fixes**: AI suggests and applies code improvements
- **Review Workflows**: Iterative improvement with quality thresholds
- **Multiple Output Formats**: Table, JSON, CSV, summary reports
- **CI/CD Integration**: Git hooks, pre-commit, workflow automation
- **Quality Scoring**: 0-100% compliance with configurable thresholds

## How It Works

AI-Rulez operates as a **comprehensive AI development governance platform**:

1. **📝 Configuration Phase**: Your `ai-rulez.yml` serves as the single source of truth
2. **🏗️ Generation Phase**: Automatically creates native files for every AI tool
3. **🔍 Enforcement Phase**: AI agents continuously validate code against your rules
4. **🛠️ Fix Phase**: Automatic corrections and improvements applied in real-time
5. **📊 Reporting Phase**: Detailed compliance reports and quality metrics

Think of it as **CI/CD for code quality** - proactive governance instead of reactive fixes.

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
    command: "npx"
    args: ["-y", "ai-rulez@latest", "mcp"]
    description: "AI-Rulez MCP server for configuration management"
```

Run `ai-rulez generate` → get all your configuration files, perfectly synchronized.

## ⚡ Quick Start

### 📋 Configuration Management
```bash
# 1. AI-powered project analysis and setup
npx ai-rulez@latest init "My Project" --preset popular

# 2. Generate all AI tool configuration files
npx ai-rulez@latest generate

# 3. Your AI tools now have comprehensive, project-specific context!
```

### 🤖 AI-Powered Rule Enforcement
```bash
# 1. Check for rule violations
uvx ai-rulez@latest enforce --agent claude --format table

# 2. Get detailed analysis and suggestions
uvx ai-rulez@latest enforce --agent claude --format json

# 3. Apply automatic fixes
uvx ai-rulez@latest enforce --agent claude --fix

# 4. Set up review workflow
uvx ai-rulez@latest enforce --agent claude --review --review-iterations 2
```

**That's it!** You now have both intelligent configuration management AND real-time rule enforcement powered by AI.

---

## 🗂️ .gitignore Management

AI-Rulez manages your `.gitignore` to keep generated files out of version control.

**Automatic updates:**
```bash
# During init (enabled by default)
ai-rulez init --preset claude

# During generate (optional)
ai-rulez generate --update-gitignore

# Disable if needed
ai-rulez init --preset popular --no-gitignore
```

**What gets ignored:**
- Generated markdown files (`CLAUDE.md`, `GEMINI.md`, `AGENTS.md`)
- AI tool directories (`.claude/`, `.cursor/`, `.windsurf/`, `.clinerules/`, etc.)
- Config files (`.mcp.json`, `.gemini/settings.json`)

All entries are added under `# AI Rules generated files` without duplicates.

**Best practice:** Commit `ai-rulez.yaml` and `.gitignore`, but not the generated AI configuration files.

---

## 🤖 AI-Powered Rule Enforcement

The killer feature that sets AI-Rulez apart: **real-time rule enforcement using AI agents**. No more manual code reviews for basic violations—let AI catch and fix issues automatically.

### ✨ Key Benefits

- **Proactive Quality Control**: Catch issues before they reach production
- **Multi-Agent Support**: Claude, Gemini, AMP, Cursor, Codex—use the best AI for each task
- **Automatic Fixes**: AI doesn't just find problems, it solves them
- **Workflow Integration**: Git hooks, CI/CD, pre-commit—enforcement everywhere
- **Team Consistency**: Same standards for everyone, from junior to senior devs

### 🎯 Real-World Use Cases

```bash
# Prevent console.log in production builds
ai-rulez enforce --agent claude --only-rules "no-console-output" --fix

# Ensure all functions have proper error handling
ai-rulez enforce --agent gemini --level strict --format json

# Multi-agent review for critical code changes
ai-rulez enforce --agent claude --review --review-agent gemini --review-threshold 95

# Automated fixes in CI/CD pipeline
ai-rulez enforce --agent claude --fix --format csv --output violations.csv
```

### 📊 Output Formats & Integration

| Format | Use Case | Command |
|--------|----------|---------|
| **Table** | Human-readable terminal output | `--format table` |
| **JSON** | API integration, detailed analysis | `--format json --pretty` |
| **CSV** | Data analysis, reporting | `--format csv --output report.csv` |
| **Summary** | Quick overview with scores | `--format summary` |

### 🔄 Advanced Workflows

**Review Pipeline**: Multi-agent validation with quality gates
```bash
ai-rulez enforce --agent claude --review --review-iterations 3 --review-threshold 85
```

**Fix & Verify**: Apply fixes and validate improvements
```bash
ai-rulez enforce --agent gemini --fix --review --require-improvement
```

**Team Standards**: Consistent enforcement across the entire codebase
```bash
ai-rulez enforce --include-files "src/**/*.{js,ts,py}" --level strict --agent claude
```

**Prefer manual setup?**
```bash
# Basic initialization without AI assistance
ai-rulez init "My Project" --preset popular --no-agent

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
    command: "npx"
    args: ["-y", "ai-rulez@latest", "mcp"]
    description: "Configuration management server"
```

## AI-Powered Rule Enforcement

AI-Rulez provides **real-time rule enforcement** using AI agents to automatically detect violations and apply fixes across your codebase.

<p align="center">
  <img src="docs/assets/ai-rulez-enforce-demo.gif" alt="AI-Rulez Enforcement Demo" width="100%">
</p>

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
- **Junie** (`junie`) - JetBrains AI assistant

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

## 📦 Installation

Choose your installation method based on your primary use case:

### 🚀 Quick Start (No Installation)

**For Configuration Management** (project setup, file generation):
```bash
# Node.js - Best for web/JS projects
npx ai-rulez@latest init "My Project" --preset popular
npx ai-rulez@latest generate
```

**For Rule Enforcement** (AI-powered validation):
```bash
# Python - Latest features, fastest updates
uvx ai-rulez@latest enforce --agent claude --fix
uvx ai-rulez@latest enforce --agent gemini --review
```

**For Go Projects**:
```bash
go run github.com/Goldziher/ai-rulez/cmd@latest init
```

### 🔧 Global Installation

For teams and frequent usage:

**Homebrew (Recommended for macOS/Linux)**
```bash
brew install goldziher/tap/ai-rulez
ai-rulez init "My Project"
ai-rulez enforce --agent claude
```

**npm (Best for Node.js teams)**
```bash
npm install -g ai-rulez
```

**pip (Best for Python teams)**
```bash
pip install ai-rulez
```

**Go (For Go developers)**
```bash
go install github.com/Goldziher/ai-rulez/cmd@latest
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
