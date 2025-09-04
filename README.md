# ai-rulez ⚡

**One config to rule them all.**

Tired of manually copying AI instructions between `.cursorrules`, `CLAUDE.md`, and a dozen other files? `ai-rulez` gives you one `ai_rulez.yaml` file that generates them all. Keep your AI context in sync across every tool you use.

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8)](https://go.dev)
[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-orange)](https://github.com/Goldziher/homebrew-tap)

### 📖 **[Read the Full Documentation](./docs/index.md)**

---

## Why ai-rulez?

- **Write once, generate everywhere:** Define your AI context in one place. `ai-rulez` creates the right format for each tool automatically.

- **CLI for everything:** Add rules, update configs, generate files—all from the command line. No more hand-editing YAML files.

- **MCP integration:** Your AI assistant can manage your configuration directly through the built-in MCP server.

- **Team-friendly:** Share configs with `extends` and `includes`. Perfect for teams and organizations.

## Future-Ready

As new AI tools emerge, `ai-rulez` grows with them. The extensible design makes it easy to add support for new assistants and platforms.

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
- **[Migration Guide](./docs/migration-guide.md)** - Upgrading from v1.x to v2.0

## Contributing

Contributions are welcome! Please see the [Contributing Guide](CONTRIBUTING.md) to get started.
