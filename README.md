# ai_rules

A CLI tool for managing AI assistant rules across different platforms.

## Features

- **Unified Configuration**: Define rules once in YAML format
- **Multiple Outputs**: Generate rules for Claude, Cursor, Windsurf, and more
- **Template System**: Customizable output templates
- **Include System**: Modular rule composition
- **Validation**: Built-in configuration validation

## Installation

### From npm (Coming Soon)
```bash
npm install -g ai_rules
```

### From pip (Coming Soon)
```bash
pip install ai_rules
```

### From Source
```bash
go install github.com/naamanhirschfeld/ai_rules@latest
```

## Quick Start

1. Initialize a new project:
```bash
ai_rules init
```

2. Edit the generated `ai_rules.yaml` file

3. Generate rule files:
```bash
ai_rules generate
```

## Configuration Format

```yaml
metadata:
  name: "My Project Rules"
  version: "1.0.0"
  description: "Rules for my React TypeScript project"

includes:
  - "./rules/react.yaml"
  - "./rules/typescript.yaml"

outputs:
  - format: "claude"
    file: "CLAUDE.md"
  - format: "cursor"
    file: ".cursorrules"

rules:
  - name: "code_style"
    priority: "high"
    content: |
      Use TypeScript strict mode
      Prefer functional components
      Use ESLint and Prettier
```

## Commands

- `ai_rules init` - Initialize a new project
- `ai_rules generate` - Generate rule files
- `ai_rules validate` - Validate configuration

## Development

### Prerequisites
- Go 1.24+
- golangci-lint
- lefthook (optional, for git hooks)

### Setup
```bash
go mod download
lefthook install  # optional
```

### Testing
```bash
go test -v ./...
```

### Linting
```bash
golangci-lint run
```

## License

MIT