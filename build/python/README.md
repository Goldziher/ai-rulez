# ai-rulez

CLI tool for managing AI assistant rules across Claude, Cursor, Windsurf and other AI-powered development environments.

This Python package provides the `ai-rulez` command-line tool, which is written in Go for optimal performance and distributed as platform-specific binaries.

## Installation

```bash
pip install ai-rulez
```

The package will automatically download the appropriate binary for your platform during installation.

## Usage

Create an `ai_rules.yaml` configuration file in your project:

```yaml
metadata:
  name: my-project
  version: 1.0.0

rules:
  - name: code-style
    content: Follow the project's established coding conventions
  - name: testing
    content: Write comprehensive tests for all new features

outputs:
  - file: .cursorrules
  - file: CLAUDE.md
```

Then generate your AI assistant configuration files:

```bash
# Generate files
ai-rulez generate

# Validate configuration
ai-rulez validate

# Initialize a new configuration
ai-rulez init
```

## Platform Support

Pre-built binaries are available for:
- macOS (Intel and Apple Silicon)
- Linux (x64, ARM64, and x86)
- Windows (x64 and x86)

## Documentation

For complete documentation, examples, and source code, visit the [GitHub repository](https://github.com/Goldziher/ai-rulez).

## License

MIT