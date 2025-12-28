# AI-Rulez Documentation

AI-Rulez is a CLI tool for managing AI assistant configurations across multiple tools.

Write your AI instructions once in a single configuration and generate tool-specific outputs for Claude, Cursor, Windsurf, Copilot, Gemini, and others.

## Core Concepts

### Directory-Based Configuration

Your configuration lives in `.ai-rulez/` with organized subdirectories:
- **rules/**: Mandatory constraints and standards
- **context/**: Reference documentation and architecture
- **skills/**: Specialized AI prompts for specific roles
- **domains/**: Team or subsystem-specific content
- **config.yaml**: Main configuration (presets, profiles)

### Presets

Presets define how content is formatted and where it's output for different tools. Built-in presets include:
- `claude` → generates `CLAUDE.md`
- `cursor` → generates `.cursor/rules/`
- `gemini` → generates `GEMINI.md`
- `copilot` → generates `.github/copilot-instructions.md`
- `windsurf` → generates `.windsurf/rules/`
- And many others...

### Profiles

Profiles let different teams generate customized outputs. Each profile specifies which domains to include:

```yaml
profiles:
  full: [backend, frontend, qa]
  backend: [backend, qa]
  frontend: [frontend, qa]
```

## Quick Navigation

### Getting Started
- **[Installation Guide](installation.md)**: Install AI-Rulez and get started
- **[Getting Started Guide](quick-start.md)**: 5-minute quick start
- **[Configuration Reference](configuration.md)**: Complete guide to all config options

### Using AI-Rulez
- **[CLI Reference](cli.md)**: All commands and flags
- **[Includes System](includes.md)**: Reusing configurations across projects
- **[Domains & Profiles](domains.md)**: Organizing by team or subsystem
- **[Custom Presets](profiles.md)**: Creating custom output formats

### Advanced Topics
- **[MCP Server](mcp-server.md)**: Exposing configuration to AI assistants
- **[Examples](examples.md)**: Real-world configuration examples
- **[Schema Reference](schema.md)**: JSON schema details
- **[Best Practices](monorepo.md)**: Patterns for large projects

## Typical Workflow

1. **Initialize** your project:
   ```bash
   ai-rulez init "my-project" --v3
   ```

2. **Add content** to `.ai-rulez/`:
   - Write rules in `rules/`
   - Add context in `context/`
   - Create skills in `skills/`

3. **Generate outputs**:
   ```bash
   ai-rulez generate
   ```

4. **Commit** the results:
   ```bash
   git add .ai-rulez/ CLAUDE.md .cursor/ GEMINI.md
   git commit -m "docs: update AI assistant guidelines"
   ```

## Key Features

- Single source of truth for all AI tool configurations
- Domain scoping to organize rules by team or subsystem
- Profile-based customization for different contexts
- Modular structure to reduce merge conflicts
- Built-in presets for major AI tools
- Custom presets for any tool and format
- MCP integration for programmatic access
- Support for monorepos and multi-team projects

## Project Structure

After initialization, your project looks like:

```
project-root/
├── .ai-rulez/
│   ├── config.yaml           # Main configuration
│   ├── rules/                # Base rules (all profiles)
│   ├── context/              # Reference docs (all profiles)
│   ├── skills/               # AI skills (all profiles)
│   ├── agents/               # Agent prompt files (all profiles)
│   └── domains/              # Team-specific content
│       ├── backend/
│       └── frontend/
├── CLAUDE.md                 # Generated for Claude
├── .cursor/rules/            # Generated for Cursor
├── GEMINI.md                 # Generated for Gemini
└── .github/copilot-instructions.md
```

## Getting Help

- **CLI Help**: `ai-rulez --help`, `ai-rulez init --help`, etc.
- **Validation**: `ai-rulez validate` to check your configuration
- **Examples**: Check the [Examples](examples.md) section
- **Issues**: Report problems on [GitHub](https://github.com/Goldziher/ai-rulez)

## Version

This documentation covers **AI-Rulez V3** (directory-based configuration).
