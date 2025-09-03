# ai-rulez ⚡

**One config to rule them all** - Generate AI assistant rules for Claude, Cursor, Windsurf, Copilot, and more from a single YAML file.

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8)](https://go.dev)
[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-orange)](https://github.com/Goldziher/homebrew-tap)

## 📖 Complete Documentation

**[Read the full documentation at ai-rulez.dev →](https://ai-rulez.dev)**

## What is ai-rulez?

Instead of maintaining separate `.cursorrules`, `CLAUDE.md`, `.github/copilot-instructions.md` files, you write **one** `ai_rulez.yaml` and generate all formats automatically.

```yaml
# ai_rulez.yaml
# extends: "https://raw.githubusercontent.com/myorg/base-config/main/typescript.yaml"  # Optional: inherit from base config

metadata:
  name: "My Project"

targets:
  claude-only:
    - "CLAUDE.md"
    - ".claude/**/*.md"

outputs:
  - path: "CLAUDE.md"
  - path: ".cursor/rules/rules.mdc"  
  - path: ".github/copilot-instructions.md"
  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{name}.md"

agents:
  - name: "code-reviewer"
    description: "Code quality and security analysis"
    targets: ["@claude-only"]
    system_prompt: |
      Senior code reviewer specializing in:
      - Code quality and maintainability
      - Security vulnerability detection
      - Performance optimization
      - Best practices enforcement

rules:
  - name: "Tech Stack"
    priority: critical
    content: |
      **Frontend**: React 19, TypeScript 5.8, Tailwind CSS
      **Backend**: Node.js, PostgreSQL, Redis
      **Testing**: Vitest, Playwright
      **Deployment**: Docker, GitHub Actions
```

Run `ai-rulez generate` → get all three files, perfectly synchronized.

## Why Use ai-rulez?

✅ **Single Source of Truth** - One YAML file generates all AI assistant formats  
✅ **Multi-Platform** - Supports 10+ AI tools (Claude, Cursor, Windsurf, Copilot, etc.)  
✅ **Monorepo Ready** - Multiple scoped configurations reduce context size and improve focus  
✅ **Built-in MCP Server** - Dynamic configuration management via Model Context Protocol  
✅ **Configuration Inheritance** - Extend base configurations with project-specific customizations  
✅ **Remote Includes** - Share configurations across projects and teams  
✅ **Flexible Composition** - Use extends + includes together for maximum configuration power  
✅ **Git Hooks Integration** - Auto-validate and generate on commit  
✅ **Agent Support** - Specialized sub-agents for different tasks (Claude)

## Supported Platforms

| Platform | Output File | Agent Support |
|----------|-------------|---------------|
| **Claude** | `CLAUDE.md` + `.claude/agents/` | ✅ Specialized agents |
| **Cursor** | `.cursor/rules/` | ✅ Shared + specific rules |
| **Windsurf** | `.windsurf/` | ✅ Directory format |
| **GitHub Copilot** | `.github/copilot-instructions.md` | ✅ Repository rules |
| **Gemini** | `GEMINI.md` | ✅ Standard format |
| **AMP/Codex** | `AGENTS.md` | ✅ Shared format |
| **Cline** | `.clinerules/` | ✅ Numbered files |
| **Continue.dev** | `.continue/rules/` | ✅ Frontmatter format |

## Quick Installation

=== "Run without installing"
```bash
# Go
go run github.com/Goldziher/ai-rulez/cmd@latest init

# Python  
uvx ai-rulez init

# Node.js
npx ai-rulez@latest init
```

=== "Install globally"
```bash
# Homebrew (macOS/Linux)
brew install goldziher/tap/ai-rulez

# Go
go install github.com/Goldziher/ai-rulez/cmd@latest

# Python
pip install ai-rulez

# Node.js  
npm install -g ai-rulez
```

## Quick Start

```bash
# 1. Create configuration
ai-rulez init "My Project"

# 2. Generate AI files  
ai-rulez generate

# 3. Validate setup
ai-rulez validate
```

## Key Features

### Multi-Platform Setup
```bash
# Popular platforms (Claude + Cursor + Windsurf + Copilot)
ai-rulez init "My Project" --popular

# Specific combinations
ai-rulez init "My Project" --claude --cursor --copilot
```

### MCP Server Integration
```bash
# Start MCP server for dynamic configuration
ai-rulez mcp

# Add to Claude Desktop config:
{
  "mcpServers": {
    "ai-rulez": {
      "command": "npx",
      "args": ["ai-rulez@latest", "mcp"]
    }
  }
}
```

### Git Hooks
```bash
# Auto-setup git hooks for validation
ai-rulez init --setup-hooks
```

## Support This Project

If you find ai-rulez helpful, please consider supporting development:

<a href="https://github.com/sponsors/Goldziher"><img src="https://img.shields.io/badge/Sponsor-%E2%9D%A4-pink?logo=github-sponsors" alt="Sponsor on GitHub" height="32"></a>

## Documentation & Examples

- **[Complete Documentation](https://ai-rulez.dev)** - Installation, configuration, examples
- **[Configuration Reference](https://ai-rulez.dev/configuration)** - All available options  
- **[MCP Server Guide](https://ai-rulez.dev/mcp-server)** - AI assistant integration
- **[CLI Commands](https://ai-rulez.dev/cli)** - Full command reference
- **[Examples](https://ai-rulez.dev/examples)** - Language-specific configurations

## Contributing

Contributions welcome! See our [Contributing Guide](CONTRIBUTING.md).
