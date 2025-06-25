# ai-rulez ⚡

> **Lightning-fast CLI tool (written in Go) for managing AI assistant rules**

Generate configuration files for Claude, Cursor, Windsurf, and other AI assistants from a single, centralized configuration.

## 🚀 Features

- ⚡ **Blazing Fast**: Written in Go for maximum performance
- 🔧 **Multi-Assistant Support**: Generate configs for Claude (CLAUDE.md), Cursor (.cursorrules), Windsurf (.windsurfrules), and more
- 📝 **Single Source of Truth**: Maintain all your AI rules in one YAML configuration
- 🎯 **Smart Templates**: Built-in templates with custom template support
- 🔍 **Validation**: Comprehensive configuration validation
- 🔄 **Git Integration**: Perfect for pre-commit hooks and CI/CD
- 📦 **Node.js Integration**: Easy installation via npm

## 📦 Installation

### npm (Recommended)

```bash
# Global installation
npm install -g ai-rulez

# Local project installation
npm install --save-dev ai-rulez
```

The npm package automatically downloads and manages the Go binary for your platform.

### Other Installation Methods

- **pip**: `pip install ai-rulez`
- **Go**: `go install github.com/Goldziher/ai-rulez@latest`
- **Homebrew**: `brew install goldziher/tap/ai-rulez` *(coming soon)*
- **Direct Download**: Download from [GitHub Releases](https://github.com/Goldziher/ai-rulez/releases)

## 🎯 Quick Start

1. **Create a configuration file** (`ai-rulez.yaml`):

```yaml
metadata:
  name: "My AI Rules"
  version: "1.0.0"

rules:
  - name: "Code Style"
    priority: 10
    content: |
      - Use TypeScript strict mode
      - Prefer functional components
      - Use meaningful variable names

  - name: "Testing"
    priority: 5
    content: |
      - Write unit tests for all functions
      - Use describe/it pattern
      - Aim for 80% code coverage

outputs:
  - file: "CLAUDE.md"
    template: "claude"
  - file: ".cursorrules"
    template: "cursor"
  - file: ".windsurfrules" 
    template: "windsurf"
```

2. **Generate configuration files**:

```bash
ai-rulez generate
```

This creates `CLAUDE.md`, `.cursorrules`, and `.windsurfrules` with your rules properly formatted for each AI assistant.

## 🛠️ Commands

```bash
# Generate all configuration files
ai-rulez generate

# Validate configuration
ai-rulez validate

# Generate recursively in subdirectories
ai-rulez generate --recursive

# Preview output without writing files
ai-rulez generate --dry-run

# Show help
ai-rulez --help
```

## 🔄 Git Integration

### Pre-commit Hook

Add to your `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v1.0.0
    hooks:
      - id: ai-rulez-generate
```

### Lefthook

Add to your `lefthook.yml`:

```yaml
pre-commit:
  commands:
    ai-rulez:
      run: ai-rulez generate
      files: git diff --cached --name-only
      glob: "*.{ai-rulez,ai_rulez}.{yml,yaml}"
```

### npm Scripts

Add to your `package.json`:

```json
{
  "scripts": {
    "ai-rulez": "ai-rulez generate",
    "ai-rulez:validate": "ai-rulez validate",
    "ai-rulez:watch": "ai-rulez generate --recursive"
  }
}
```

## 📚 Configuration

The tool looks for configuration files in this order:
- `.ai-rulez.yaml`
- `ai-rulez.yaml` 
- `.ai_rulez.yaml`
- `ai_rulez.yaml`

### Configuration Schema

```yaml
metadata:
  name: string          # Required: Project name
  version: string       # Required: Version
  description: string   # Optional: Description

rules:
  - name: string        # Required: Rule name
    priority: number    # Required: Priority (1-10)
    content: string     # Required: Rule content

sections:              # Optional: Organize rules into sections
  - title: string      # Required: Section title
    priority: number   # Required: Section priority
    content: string    # Required: Section content

outputs:               # Required: At least one output
  - file: string       # Required: Output filename
    template: string   # Required: Template name or path

includes:              # Optional: Include other config files
  - path/to/other.yaml
```

## 🎨 Templates

Built-in templates:
- `claude` - CLAUDE.md format
- `cursor` - .cursorrules format  
- `windsurf` - .windsurfrules format
- `default` - Generic format

Custom templates use Go template syntax with access to `.Rules`, `.Sections`, `.Metadata`, etc.

## 🔧 Advanced Usage

### Environment Variables

- `AI_RULEZ_CONFIG` - Override config file path
- `AI_RULEZ_DEBUG` - Enable debug output

### Node.js API

```javascript
const { execSync } = require('child_process');

// Run ai-rulez programmatically
try {
  const output = execSync('ai-rulez generate --dry-run', { encoding: 'utf8' });
  console.log(output);
} catch (error) {
  console.error('ai-rulez failed:', error.message);
}
```

### npm Scripts Integration

```json
{
  "scripts": {
    "precommit": "ai-rulez generate",
    "lint": "eslint . && ai-rulez validate",
    "build": "npm run ai-rulez && npm run compile"
  },
  "husky": {
    "hooks": {
      "pre-commit": "ai-rulez generate"
    }
  }
}
```

## 🤝 Contributing

Contributions are welcome! Please see our [Contributing Guide](https://github.com/Goldziher/ai-rulez/blob/main/CONTRIBUTING.md).

## 📄 License

MIT License - see [LICENSE](https://github.com/Goldziher/ai-rulez/blob/main/LICENSE)

## 🔗 Links

- [GitHub Repository](https://github.com/Goldziher/ai-rulez)
- [Documentation](https://github.com/Goldziher/ai-rulez#readme)
- [Issues](https://github.com/Goldziher/ai-rulez/issues)
- [Releases](https://github.com/Goldziher/ai-rulez/releases)
- [PyPI Package](https://pypi.org/project/ai-rulez/)

---

**Note**: This npm package is a wrapper around the Go binary. The actual tool is written in Go for maximum performance and cross-platform compatibility.