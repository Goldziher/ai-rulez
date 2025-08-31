# ai-rulez ⚡

Lightning-fast CLI tool and MCP server for managing AI assistant rules across Claude, Cursor, Windsurf, Copilot, and other AI tools from a single YAML configuration.

## What is ai-rulez?

ai-rulez generates AI assistant instruction files from a single configuration. Instead of maintaining separate `.cursorrules`, `CLAUDE.md`, `.github/copilot-instructions.md` files, you write one `ai_rulez.yaml` and generate all formats.

## Quick Example

Create an `ai_rulez.yaml`:

```yaml
metadata:
  name: "My Project"

rules:
  - name: "code-style"
    content: "Use TypeScript with strict mode"
    priority: 10

outputs:
  - path: "CLAUDE.md"
    template: "claude"
  - path: ".cursor/rules/"
    template: "cursor"
```

Run `ai-rulez generate` and get both files automatically generated and kept in sync.

## Key Features

- **Single Source of Truth** - One YAML file generates all AI assistant formats
- **Multiple AI Platforms** - Claude, Cursor, Windsurf, Copilot, Gemini, and more
- **MCP Server** - Built-in Model Context Protocol server for dynamic configuration
- **Git Hooks Integration** - Automatic validation with lefthook, pre-commit, or husky
- **Remote Includes** - Share configurations across projects
- **Agent Support** - Specialized sub-agents for different tasks

## Supported AI Platforms

| Platform | Output | Format |
|----------|--------|---------|
| Claude | `CLAUDE.md` | Markdown |
| Claude Agents | `.claude/agents/` | Directory with specialized agents |
| Cursor | `.cursor/rules/` | MDC files |
| Windsurf | `.windsurf/` | Markdown files |
| GitHub Copilot | `.github/copilot-instructions.md` | Markdown |
| Gemini | `GEMINI.md` | Markdown |
| Cline | `.clinerules/` | Numbered markdown files |
| Continue.dev | `.continue/rules/` | Frontmatter markdown |
| AMP/Codex | `AGENTS.md` | Shared format |

## Installation

=== "Homebrew"
    ```bash
    brew install goldziher/tap/ai-rulez
    ```

=== "Go"
    ```bash
    go install github.com/Goldziher/ai-rulez/cmd@latest
    ```

=== "npm"
    ```bash
    npm install -g ai-rulez
    ```

=== "pip"
    ```bash
    pip install ai-rulez
    ```

=== "Run without installing"
    ```bash
    # Go
    go run github.com/Goldziher/ai-rulez/cmd@latest --help
    
    # Python
    uvx ai-rulez --help
    
    # Node.js
    npx ai-rulez@latest --help
    ```

## Next Steps

1. [Install ai-rulez](installation.md)
2. [Follow the Quick Start guide](quick-start.md)
3. [Learn about Configuration](configuration.md)
4. [Browse Examples](examples.md)