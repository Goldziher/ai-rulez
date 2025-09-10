# ai-rulez ⚡

<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/logo.png" alt="ai-rulez logo" width="200" style="border-radius: 15%; overflow: hidden;">
</p>

**One config to rule them all.**

Tired of manually managing rule files, subagents, and custom commands across different AI tools? `ai-rulez` gives you one `ai-rulez.yml` file to generate them all. Keep your AI context in sync, and even launch MCP servers for direct integration.

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8)](https://go.dev)
[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-orange)](https://github.com/Goldziher/homebrew-tap)

### 📖 **[Read the Full Documentation](https://goldziher.github.io/ai-rulez/)**

---

## Feature Highlights

`ai-rulez` is a progressive tool-box, designed to offer a centralized way to manage AI tooling for a repository.

### Centralized Configuration
- **Centralized Definitions:** Use a single `ai-rulez.yml` as the source of truth to define rules, file structures, and documentation for all your AI tools.
- **Nested Configs & Monorepo Support:** Scale your configurations with `extends` and `includes`. Manage complex projects and monorepos with ease by using the `--recursive` flag to combine configurations from multiple files.
  ```bash
  # Generate rules for all projects in a monorepo
  ai-rulez generate --recursive
  ```

### Powerful Tooling
- **Custom Commands:** Define custom commands that your AI assistant can execute, enabling powerful, interactive workflows.
- **Specialized AI Agents:** Create specialized "sub-agents" with their own system prompts and tools, perfect for complex tasks like code reviews or database queries.
- **MCP Integration:** Automatically configure MCP servers across CLI tools (Claude, Gemini) and generate config files for others (Cursor, VS Code). One configuration, every tool connected.
- **Full-Featured CLI:** Manage your entire configuration from the command line. Add rules, update agents, and generate files without ever opening a YAML file.

### Flexible Integrations
- **Multi-Tool Support:** Use presets to instantly generate configurations for popular AI tools like Claude, Cursor, Copilot, Gemini, and more.
- **Custom Tool Integration:** Don't see your favorite tool on the list? Use the `outputs` key to generate a configuration file for any tool, in any format.

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
  - "popular"  # Includes Claude, Cursor, Windsurf, and Copilot

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
    rev: v2.1.0
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
