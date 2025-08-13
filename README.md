# ai-rulez ⚡

Lightning-fast CLI for managing AI assistant rules across Claude, Cursor, Windsurf, and other tools from a single YAML config.

## Installation

### using Go

```shell
go install github.com/Goldziher/ai-rulez@latest
```

### using pip

```shell
pip install ai-rulez
```

### using npm

```shell
npm i -g ai-rulez
```
### using Homebrew

```shell
brew install goldziher/tap/ai-rulez
```

## Quick Start

```bash
# 1. Initialize your project
ai-rulez init

# 2. Edit ai-rulez.yaml to add your rules

# 3. Generate assistant configs
ai-rulez generate
```

That's it! Your rules are now available in the configured file formats

## Basic Configuration

```yaml
# ai-rulez.yaml
metadata:
  name: "My Project"
  version: "1.0.0"

outputs:
  - path: "CLAUDE.md"
  - path: ".cursor/rules/"
    type: "rule"
    naming_scheme: "rules.mdc"
  - path: ".windsurfrules"

rules:
  - name: "Code Style"
    priority: 10
    content: |
      - Use TypeScript strict mode
      - Prefer functional components

  - name: "Testing"
    priority: 5
    content: |
      - Write unit tests for all functions
      - Aim for 80% coverage
```

## Core Commands

```bash
ai-rulez init                # Initialize config
ai-rulez generate            # Generate outputs
ai-rulez validate            # Validate config
ai-rulez generate --dry-run  # Preview changes
```

## Key Features

- **Single source of truth** - One YAML file for all AI assistants
- **Smart templates** - Built-in templates for each assistant
- **Remote includes** - Share rules via URLs (GitHub, APIs)
- **Local overrides** - Personal customization with `.local.yaml`
- **MCP Server** - Built-in Model Context Protocol server integration
- **Fast** - Written in Go, only writes when content changes

## Advanced Usage

### Share Rules Across Projects

```yaml
includes:
  - "https://raw.githubusercontent.com/myorg/standards/main/common.yaml"
  - "./local-overrides.yaml"
```

### Personal Preferences (`.local.yaml`)

```yaml
# ai-rulez.local.yaml (add to .gitignore)
rules:
  - name: "Communication Style"
    content: "Be concise. Use bullet points."
```

### Directory Outputs

```yaml
outputs:
  - path: ".claude/agents/"  # Note trailing slash
    type: "agent"
    naming_scheme: "{name}.md"
```

### MCP Server Integration

```bash
# Start MCP server for Claude Desktop integration
npx ai-rulez mcp

# Add to Claude Desktop configuration:
# "ai-rulez": { "command": "npx", "args": ["ai-rulez@latest", "mcp"] }
```

### Pre-commit Hooks

```yaml
# .pre-commit-config.yaml
repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v1.4.2
    hooks:
      - id: ai-rulez-validate
```

## Documentation

- [Configuration Schema](https://github.com/Goldziher/ai-rulez/blob/main/schema/ai-rules-v1.schema.json)
- [Examples](https://github.com/Goldziher/ai-rulez/tree/main/examples)
- [Contributing](https://github.com/Goldziher/ai-rulez/blob/main/CONTRIBUTING.md)
