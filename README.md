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

`ai-rulez` is a simple and  than just a configuration generator. It's a complete system for managing your AI's context.

-   **Single Source of Truth:** Define your project's context—rules, file structures, and key documentation—in one `ai-rulez.yml` file.
-   **Multi-Tool Support:** Use presets to instantly generate configurations for popular AI tools like Claude, Cursor, Copilot, Gemini, and more.
-   **Custom Tool Integration:** Don't see your favorite tool on the list? Use the `outputs` key to generate a configuration file for any tool, in any format.
-   **Specialized AI Agents:** Create specialized "sub-agents" with their own system prompts and tools, perfect for complex tasks like code reviews or database queries.
-   **Composable Configuration:** Use `extends` and `includes` to build powerful, maintainable configurations that can be shared across teams and projects.
-   **Full-Featured CLI:** Manage your entire configuration from the command line. Add rules, update agents, and generate files without ever opening a YAML file.
-   **MCP Server Integration:** Allow your AI assistant to programmatically interact with your configuration through the built-in Model Context Protocol (MCP) server.

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
```

Run `ai-rulez generate` → get all your configuration files, perfectly synchronized.

## Quick Start

```bash
# 1. Initialize your project with a preset (recommended)
ai-rulez init "My Project" --preset popular

# 2. Add your project-specific context
ai-rulez add rule "Tech Stack" --priority critical --content "This project uses Go and PostgreSQL."

# 3. Generate all AI instruction files
ai-rulez generate
```

## Installation

### Run without installing

For one-off executions, you can run `ai-rulez` directly without a system-wide installation.

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

---

## Documentation

- **[Quick Start Guide](https://goldziher.github.io/ai-rulez/quick-start/)**
- **[Full CLI Reference](https://goldziher.github.io/ai-rulez/cli/)**
- **[Configuration Guide](https://goldziher.github.io/ai-rulez/configuration/)**
- **[Migration Guide](https://goldziher.github.io/ai-rulez/migration-guide/)** - Upgrading from v1.x to v2.0

## Contributing

Contributions are welcome! Please see the [Contributing Guide](CONTRIBUTING.md) to get started.
