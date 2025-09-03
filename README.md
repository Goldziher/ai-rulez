# ai-rulez ⚡

**One config to rule them all.**

Stop manually synchronizing AI instructions across dozens of files. `ai-rulez` provides a single, elegant `ai_rulez.yaml` to manage context for Claude, Cursor, Copilot, and all your other AI tools, with a powerful CLI and programmatic API to automate everything.

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8)](https://go.dev)
[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-orange)](https://github.com/Goldziher/homebrew-tap)

### 📖 **[Read the Full Documentation](./docs/index.md)**

---

## Why ai-rulez?

- **Single Source of Truth:** Define rules, agents, and context once. Generate everywhere. Eliminate context drift and ensure all your AI tools operate with the same instructions.

- **Complete CLI Toolkit:** Manage your entire configuration from the command line. Add a rule, update an agent, or manage include files programmatically. Never hand-edit YAML again.

- **Full Programmatic API:** A built-in MCP Server exposes the entire `ai-rulez` feature set, allowing for deep, programmatic integration with AI assistants and custom tools.

- **Built for Scale:** Use `extends` and `includes` to create composable, reusable configurations that can be shared across your entire organization, ensuring consistency while maintaining flexibility.

## Future-Facing by Design

The AI landscape is evolving rapidly. `ai-rulez` is designed to be an extensible, future-facing platform. Its provider-based architecture and powerful templating system make it simple to add support for new AI assistants, agentic tools, and novel configuration formats as they emerge.

## Supported Tools & Platforms

`ai-rulez` includes presets for a wide range of popular AI development tools:

- **Anthropic Claude** (including Claude Code)
- **Cursor**
- **GitHub Copilot**
- **Google Gemini**
- **Sourcegraph Cody & AMP**
- **Continue.dev**
- **Windsurf**
- **Cline**

## Example: `ai_rulez.yaml`

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

# Inherit from a company-wide base configuration
extends: "https://example.com/base-config.yaml"

metadata:
  name: "My Project"

outputs:
  - path: "CLAUDE.md"
  - path: ".cursor/rules/"
    type: "rule"
    naming_scheme: "{name}.mdc"

agents:
  - name: "code-reviewer"
    description: "Code quality and security analysis specialist."
    system_prompt: "You are a senior code reviewer..."

rules:
  - name: "Tech Stack"
    priority: critical
    content: "Frontend: React, TypeScript. Backend: Go, PostgreSQL."
    # Target this rule to specific output files
    targets: ["CLAUDE.md"]
```

Run `ai-rulez generate` → get all your configuration files, perfectly synchronized.

## Quick Start

```bash
# 1. Initialize your project with sane defaults
ai-rulez init "My Project" --popular

# 2. Add your project-specific context
ai-rulez add rule "Tech Stack" --priority critical --content "This project uses Go and PostgreSQL."

# 3. Generate all AI instruction files
ai-rulez generate
```

## Installation

=== "Run without installing"
    ```bash
    # Node.js
    npx ai-rulez@latest init

    # Python
    uvx ai-rulez init
    ```

=== "Install globally"
    ```bash
    # Homebrew (macOS/Linux)
    brew install goldziher/tap/ai-rulez

    # npm
    npm install -g ai-rulez

    # pip
    pip install ai-rulez
    ```

---

## Documentation

- **[Quick Start Guide](./docs/quick-start.md)**
- **[Full CLI Reference](./docs/cli.md)**
- **[Configuration Guide](./docs/configuration.md)**
- **[Best Practices](./docs/best-practices.md)**

## Contributing

Contributions are welcome! Please see the [Contributing Guide](CONTRIBUTING.md) to get started.
