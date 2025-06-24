# airules

A CLI tool for managing AI assistant rules with modular configuration and template support.

## Installation

```bash
npm install -g airules
```

## Quick Start

1. Initialize a new configuration:
```bash
airules init
```

2. Generate AI assistant rule files:
```bash
airules generate
```

This creates rule files for Claude, Cursor, Windsurf, and other AI assistants based on your configuration.

## Features

- **Unified Configuration**: Define rules once in YAML format
- **Multiple Outputs**: Generate rules for Claude, Cursor, Windsurf, and any AI assistant
- **Template System**: Built-in and custom templates
- **Modular Composition**: Include system for organizing rules
- **Smart Generation**: Only writes files when content changes

## Configuration

Create a configuration file (supports multiple names):
- `airules.yaml` or `.airules.yaml`
- `ai_rules.yaml` or `.ai_rules.yaml`
- `.yml` extension also supported

Example configuration:

```yaml
metadata:
  name: My Project
  version: 1.0.0
  description: AI assistant rules for my project

outputs:
  - file: claude.md
  - file: .cursorrules
  - file: .windsurfrules

rules:
  - name: Code Style
    priority: 10
    content: Follow the project's established code style and conventions

  - name: Error Handling
    priority: 8
    content: Always handle errors appropriately and provide meaningful error messages

  - name: Documentation
    priority: 5
    content: Document all public APIs and complex logic
```

## Commands

- `airules init` - Initialize a new configuration
- `airules generate` - Generate AI assistant rule files
- `airules validate` - Validate configuration syntax
- `airules --version` - Show version information

## Documentation

For full documentation, examples, and advanced features, visit:
https://github.com/Goldziher/airules

## About

`airules` is written in Go for performance and distributed as a single binary through npm for easy installation. No Go installation required - npm handles downloading the appropriate binary for your platform.

## License

MIT